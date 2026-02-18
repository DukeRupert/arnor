package peon

import (
	_ "embed"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/ssh"
)

//go:embed peon.sh
var script []byte

const keyDelimiter = "──────────────────────────────────────────────"

// RunRemote connects to the remote host via SSH and executes the embedded
// peon.sh bootstrap script. It returns the peon private key extracted from
// the script output.
func RunRemote(host, user, sshPass, sudoPass string) (string, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(sshPass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := host
	if !strings.Contains(host, ":") {
		addr = host + ":22"
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return "", fmt.Errorf("SSH connect failed: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session failed: %w", err)
	}
	defer session.Close()

	// Build stdin: for non-root users, pipe sudo password first
	var stdin io.Reader
	if user == "root" {
		stdin = strings.NewReader(string(script))
	} else {
		stdin = io.MultiReader(
			strings.NewReader(sudoPass+"\n"),
			strings.NewReader(string(script)),
		)
	}
	session.Stdin = stdin

	// Build the command
	var cmd string
	if user == "root" {
		cmd = "bash -s"
	} else {
		cmd = "sudo -S bash -s"
	}

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("remote execution failed: %w\nOutput:\n%s", err, string(output))
	}

	key, err := extractPrivateKey(string(output))
	if err != nil {
		return "", fmt.Errorf("%w\nFull output:\n%s", err, string(output))
	}

	return key, nil
}

// extractPrivateKey finds the private key block between the delimiter lines
// in the peon.sh output.
func extractPrivateKey(output string) (string, error) {
	parts := strings.Split(output, keyDelimiter)
	if len(parts) < 3 {
		return "", fmt.Errorf("could not find private key delimiters in output")
	}
	// The key is between the first and second delimiter occurrence
	key := strings.TrimSpace(parts[1])
	if key == "" {
		return "", fmt.Errorf("private key block is empty")
	}
	return key, nil
}
