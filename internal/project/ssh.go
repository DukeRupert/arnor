package project

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHResult contains output from SSH setup commands.
type SSHResult struct {
	DeployPublicKey string
}

// RunSetup connects to the VPS as peon and creates the deploy user,
// deploy path, docker group membership, and SSH keypair.
func RunSetup(serverIP, deployUser, deployPath, peonKeyPEM string) (*SSHResult, error) {
	signer, err := ssh.ParsePrivateKey([]byte(peonKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("parsing peon SSH key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: "peon",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", serverIP+":22", config)
	if err != nil {
		return nil, fmt.Errorf("SSH dial to %s: %w", serverIP, err)
	}
	defer client.Close()

	// Create deploy user with home dir and docker group
	commands := []string{
		fmt.Sprintf("sudo useradd -m -s /bin/bash %s 2>/dev/null || true", deployUser),
		fmt.Sprintf("sudo usermod -aG docker %s 2>/dev/null || true", deployUser),
		fmt.Sprintf("sudo mkdir -p %s", deployPath),
		fmt.Sprintf("sudo chown %s:%s %s", deployUser, deployUser, deployPath),
		// Generate SSH keypair for the deploy user
		fmt.Sprintf("sudo mkdir -p /home/%s/.ssh", deployUser),
		fmt.Sprintf("sudo ssh-keygen -t ed25519 -f /home/%s/.ssh/id_ed25519 -N '' -q <<< y 2>/dev/null || true", deployUser),
		fmt.Sprintf("sudo cp /home/%s/.ssh/id_ed25519.pub /home/%s/.ssh/authorized_keys", deployUser, deployUser),
		fmt.Sprintf("sudo chown -R %s:%s /home/%s/.ssh", deployUser, deployUser, deployUser),
		fmt.Sprintf("sudo chmod 700 /home/%s/.ssh", deployUser),
		fmt.Sprintf("sudo chmod 600 /home/%s/.ssh/authorized_keys", deployUser),
	}

	for _, c := range commands {
		if err := runSSHCommand(client, c); err != nil {
			return nil, fmt.Errorf("running %q: %w", c, err)
		}
	}

	// Read the generated private key to return for GitHub secrets
	keyOutput, err := runSSHCommandOutput(client, fmt.Sprintf("sudo cat /home/%s/.ssh/id_ed25519", deployUser))
	if err != nil {
		return nil, fmt.Errorf("reading deploy key: %w", err)
	}

	return &SSHResult{
		DeployPublicKey: strings.TrimSpace(keyOutput),
	}, nil
}

func runSSHCommand(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	return session.Run(command)
}

func runSSHCommandOutput(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	out, err := session.Output(command)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// hostPortRe matches host-side port bindings in docker ps output, e.g. "0.0.0.0:3000->3000/tcp".
var hostPortRe = regexp.MustCompile(`:(\d+)->`)

// GetUsedPorts SSHs into the server as peon and returns the host-side ports
// currently bound by Docker containers.
func GetUsedPorts(serverIP, peonKeyPEM string) ([]int, error) {
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
	defer client.Close()

	output, err := runSSHCommandOutput(client, "docker ps --format '{{.Ports}}'")
	if err != nil {
		return nil, fmt.Errorf("running docker ps: %w", err)
	}

	seen := make(map[int]bool)
	for _, match := range hostPortRe.FindAllStringSubmatch(output, -1) {
		port, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		seen[port] = true
	}

	ports := make([]int, 0, len(seen))
	for p := range seen {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports, nil
}

// SuggestPort returns the lowest port starting from 3000 that is not in usedPorts.
func SuggestPort(usedPorts []int) int {
	used := make(map[int]bool, len(usedPorts))
	for _, p := range usedPorts {
		used[p] = true
	}
	for port := 3000; ; port++ {
		if !used[port] {
			return port
		}
	}
}
