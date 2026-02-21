package caddy

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// InstallParams contains all inputs for Caddy installation on a remote server.
type InstallParams struct {
	ServerIP   string
	PeonKeyPEM string
	CFToken    string // empty = skip systemd override
	OnProgress func(step, total int, message string)
}

const caddyDownloadURL = "https://caddyserver.com/api/download?os=linux&arch=amd64&p=github.com/caddy-dns/cloudflare"

const caddyServiceUnit = `[Unit]
Description=Caddy
Documentation=https://caddyserver.com/docs/
After=network.target network-online.target
Requires=network-online.target

[Service]
Type=notify
User=caddy
Group=caddy
ExecStart=/usr/bin/caddy run --environ --config /etc/caddy/Caddyfile
ExecReload=/usr/bin/caddy reload --config /etc/caddy/Caddyfile --force
TimeoutStopSec=5s
LimitNOFILE=1048576
LimitNPROC=512
PrivateTmp=true
ProtectSystem=full
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
`

const caddyfile = `{
	log {
		output file /var/log/caddy/access.log
	}
}

import conf.d/*
`

const cfOverride = `[Service]
Environment="CF_API_TOKEN=%s"
`

// Install SSHes into a server as peon and sets up Caddy with the cloudflare DNS module.
func Install(params InstallParams) error {
	const totalSteps = 8
	report := func(step int, message string) {
		if params.OnProgress != nil {
			params.OnProgress(step, totalSteps, message)
		}
	}

	client, err := dialPeon(params.ServerIP, params.PeonKeyPEM)
	if err != nil {
		return err
	}
	defer client.Close()

	// Step 1: Download custom Caddy binary
	report(1, "Downloading Caddy with cloudflare module...")
	if err := sshRun(client, fmt.Sprintf("curl -fsSL -o /tmp/caddy '%s'", caddyDownloadURL)); err != nil {
		return fmt.Errorf("downloading caddy: %w", err)
	}

	// Step 2: Create caddy system user
	report(2, "Creating caddy user...")
	if err := sshRun(client, "id caddy >/dev/null 2>&1 || sudo useradd --system --home /var/lib/caddy --shell /usr/sbin/nologin caddy"); err != nil {
		return fmt.Errorf("creating caddy user: %w", err)
	}

	// Step 3: Stop caddy, install binary
	report(3, "Installing Caddy binary...")
	cmds := []string{
		"sudo systemctl stop caddy 2>/dev/null || true",
		"sudo mv /tmp/caddy /usr/bin/caddy",
		"sudo chmod 755 /usr/bin/caddy",
	}
	for _, c := range cmds {
		if err := sshRun(client, c); err != nil {
			return fmt.Errorf("installing caddy binary: %w", err)
		}
	}

	// Step 4: Write systemd unit if missing
	report(4, "Setting up systemd service...")
	if err := sshRun(client, "test -f /etc/systemd/system/caddy.service || test -f /lib/systemd/system/caddy.service"); err != nil {
		// No unit file exists, write one
		if err := sshWriteFile(client, "/etc/systemd/system/caddy.service", caddyServiceUnit); err != nil {
			return fmt.Errorf("writing caddy.service: %w", err)
		}
	}

	// Step 5: Create /etc/caddy/conf.d/ directory and write Caddyfile
	report(5, "Writing Caddyfile...")
	if err := sshRun(client, "sudo mkdir -p /etc/caddy/conf.d"); err != nil {
		return fmt.Errorf("creating conf.d: %w", err)
	}
	// Write Caddyfile only if it doesn't already contain the import directive
	existing, _ := sshOutput(client, "cat /etc/caddy/Caddyfile 2>/dev/null || true")
	if !strings.Contains(existing, "import conf.d/*") {
		if err := sshWriteFile(client, "/etc/caddy/Caddyfile", caddyfile); err != nil {
			return fmt.Errorf("writing Caddyfile: %w", err)
		}
	}

	// Step 6: Create log directory
	report(6, "Creating log directory...")
	if err := sshRun(client, "sudo mkdir -p /var/log/caddy && sudo chown caddy:caddy /var/log/caddy"); err != nil {
		return fmt.Errorf("creating log dir: %w", err)
	}

	// Step 7: Write CF token systemd override (if provided)
	report(7, "Configuring Cloudflare token...")
	if params.CFToken != "" {
		if err := sshRun(client, "sudo mkdir -p /etc/systemd/system/caddy.service.d"); err != nil {
			return fmt.Errorf("creating override dir: %w", err)
		}
		override := fmt.Sprintf(cfOverride, params.CFToken)
		if err := sshWriteFile(client, "/etc/systemd/system/caddy.service.d/cloudflare.conf", override); err != nil {
			return fmt.Errorf("writing cloudflare override: %w", err)
		}
	}

	// Step 8: daemon-reload + enable + restart
	report(8, "Starting Caddy...")
	startCmds := []string{
		"sudo systemctl daemon-reload",
		"sudo systemctl enable caddy",
		"sudo systemctl restart caddy",
	}
	for _, c := range startCmds {
		if err := sshRun(client, c); err != nil {
			return fmt.Errorf("starting caddy (%s): %w", c, err)
		}
	}

	return nil
}

// SSH helpers — small enough to duplicate rather than creating a shared package.

func dialPeon(serverIP, peonKeyPEM string) (*ssh.Client, error) {
	signer, err := ssh.ParsePrivateKey([]byte(peonKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("parsing peon SSH key: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            "peon",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", serverIP+":22", config)
	if err != nil {
		return nil, fmt.Errorf("SSH dial to %s: %w", serverIP, err)
	}
	return client, nil
}

func sshRun(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	return session.Run(command)
}

func sshOutput(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	out, err := session.Output(command)
	return string(out), err
}

func sshWriteFile(client *ssh.Client, path, content string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	session.Stdin = strings.NewReader(content)
	session.Stderr = &stderr
	err = session.Run(fmt.Sprintf("sudo tee %s > /dev/null", path))
	session.Close()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("%s", errMsg)
		}
		return err
	}
	return nil
}
