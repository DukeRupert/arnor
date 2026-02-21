package service

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dukerupert/arnor/internal/caddy"
	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/dns"
	"github.com/dukerupert/arnor/internal/hetzner"
	"golang.org/x/crypto/ssh"
)

// DeployParams contains all inputs for service deployment.
type DeployParams struct {
	ServiceName string
	ServerName  string
	Domain      string
	Port        int
	ComposeFile string // local path to docker-compose.yml
	PeonKey     string // PEM-encoded peon SSH key
	Store       config.Store
	OnProgress  func(step, total int, message string)
}

// Deploy runs the full service deployment orchestration.
func Deploy(params DeployParams) error {
	const totalSteps = 8
	report := func(step int, message string) {
		if params.OnProgress != nil {
			params.OnProgress(step, totalSteps, message)
		}
	}

	cfg, err := params.Store.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Step 1: Look up server IP
	report(1, "Looking up server...")
	server := cfg.FindServer(params.ServerName)
	if server == nil {
		mgr, err := hetzner.NewManager(cfg.HetznerProjects, params.Store)
		if err != nil {
			return fmt.Errorf("creating Hetzner manager: %w", err)
		}
		s, err := mgr.GetServer(params.ServerName)
		if err != nil {
			return fmt.Errorf("server %q not found in config or Hetzner: %w", params.ServerName, err)
		}
		server = &config.Server{
			Name:           s.Name,
			IP:             s.PublicNet.IPv4.IP,
			HetznerProject: s.ProjectAlias,
			HetznerID:      s.ID,
		}
	}

	// Step 2: Detect DNS provider
	report(2, "Detecting DNS provider...")
	provider, err := dns.ProviderForDomain(params.Domain, cfg, params.Store)
	if err != nil {
		return fmt.Errorf("detecting DNS provider for %s: %w", params.Domain, err)
	}

	// Resolve peon key
	peonKey := params.PeonKey
	if peonKey == "" {
		peonKey, err = params.Store.GetPeonKey(server.IP)
		if err != nil {
			return fmt.Errorf("peon key for %s: %w", server.IP, err)
		}
	}

	// Open single SSH connection for steps 3-6
	client, err := dialPeon(server.IP, peonKey)
	if err != nil {
		return err
	}
	defer client.Close()

	deployPath := fmt.Sprintf("/opt/%s", params.ServiceName)

	// Step 3: Create deploy directory
	report(3, "Creating deploy directory...")
	cmds := []string{
		fmt.Sprintf("sudo mkdir -p %s", deployPath),
		fmt.Sprintf("sudo chown peon:peon %s", deployPath),
	}
	for _, c := range cmds {
		if err := sshRun(client, c); err != nil {
			return fmt.Errorf("creating deploy dir: %w", err)
		}
	}

	// Step 4: Upload docker-compose.yml
	report(4, "Uploading docker-compose.yml...")
	composeContent, err := os.ReadFile(params.ComposeFile)
	if err != nil {
		return fmt.Errorf("reading compose file %s: %w", params.ComposeFile, err)
	}
	composePath := deployPath + "/docker-compose.yml"
	if err := sshWriteFile(client, composePath, string(composeContent)); err != nil {
		return fmt.Errorf("uploading compose file: %w", err)
	}
	if err := sshRun(client, fmt.Sprintf("sudo chown peon:peon %s", composePath)); err != nil {
		return fmt.Errorf("chowning compose file: %w", err)
	}

	// Step 5: Run docker compose up
	report(5, "Starting containers...")
	if err := sshRun(client, fmt.Sprintf("cd %s && docker compose up -d", deployPath)); err != nil {
		return fmt.Errorf("running docker compose up: %w", err)
	}

	// Step 6: Generate and deploy Caddy config
	report(6, "Writing Caddy config...")
	caddyConfig := caddy.Generate(params.Domain, params.Port, provider.Name())
	caddyPath := fmt.Sprintf("/etc/caddy/conf.d/%s.caddy", params.Domain)
	if err := sshRun(client, "sudo mkdir -p /etc/caddy/conf.d"); err != nil {
		return fmt.Errorf("creating caddy conf.d: %w", err)
	}
	if err := sshWriteFile(client, caddyPath, caddyConfig); err != nil {
		return fmt.Errorf("writing caddy config: %w", err)
	}
	validateOut, err := sshOutput(client, "sudo caddy validate --config /etc/caddy/Caddyfile 2>&1")
	if err != nil {
		return fmt.Errorf("caddy config validation failed: %s", strings.TrimSpace(validateOut))
	}
	if err := sshRun(client, "sudo systemctl reload caddy"); err != nil {
		journalOut, _ := sshOutput(client, "sudo journalctl -u caddy -n 20 --no-pager 2>&1")
		return fmt.Errorf("reloading caddy: %w\njournal output:\n%s", err, strings.TrimSpace(journalOut))
	}

	// Step 7: Create DNS records
	report(7, "Creating DNS records...")
	rootDomain, err := config.RootDomain(params.Domain)
	if err != nil {
		return fmt.Errorf("resolving root domain for %s: %w", params.Domain, err)
	}
	subName := ""
	if rootDomain != params.Domain {
		subName = strings.TrimSuffix(params.Domain, "."+rootDomain)
	}

	// Remove existing A/CNAME/ALIAS records before creating new ones
	if existing, err := provider.ListRecords(rootDomain); err == nil {
		for _, r := range existing {
			if r.Name == params.Domain && (r.Type == "A" || r.Type == "CNAME" || r.Type == "ALIAS") {
				provider.DeleteRecord(rootDomain, r.ID)
			}
		}
	}

	if _, err := provider.CreateRecord(rootDomain, subName, "A", server.IP, "600"); err != nil {
		return fmt.Errorf("creating A record: %w", err)
	}

	// Best-effort www CNAME
	wwwName := "www"
	if subName != "" {
		wwwName = "www." + subName
	}
	provider.CreateRecord(rootDomain, wwwName, "CNAME", params.Domain, "600")

	// Step 8: Save to config
	report(8, "Updating config...")
	env := config.Environment{
		Domain:      params.Domain,
		DNSProvider: provider.Name(),
		DeployPath:  deployPath,
		DeployUser:  "peon",
		Port:        params.Port,
	}

	existingProject := cfg.FindProject(params.ServiceName)
	if existingProject != nil {
		if existingProject.Environments == nil {
			existingProject.Environments = make(map[string]config.Environment)
		}
		existingProject.Environments["prod"] = env
	} else {
		cfg.Projects = append(cfg.Projects, config.Project{
			Name:   params.ServiceName,
			Repo:   "",
			Server: params.ServerName,
			Environments: map[string]config.Environment{
				"prod": env,
			},
		})
	}

	if err := params.Store.SaveConfig(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	return nil
}

// SSH helpers — duplicated from internal/caddy/install.go per project convention.

func dialPeon(serverIP, peonKeyPEM string) (*ssh.Client, error) {
	signer, err := ssh.ParsePrivateKey([]byte(peonKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("parsing peon SSH key: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            "peon",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", serverIP+":22", sshConfig)
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
