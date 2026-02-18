package peon

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

//go:embed peon.sh
var script []byte

const keyDelimiter = "──────────────────────────────────────────────"

// PassphraseFunc is called when an SSH key requires a passphrase.
// The caller provides this to handle interactive prompting.
type PassphraseFunc func() ([]byte, error)

// SSHAuth holds the authentication parameters for RunRemote.
type SSHAuth struct {
	// Password for password-based SSH auth (empty to skip)
	Password string
	// SudoPassword for non-root users
	SudoPassword string
	// KeyPassphraseFunc is called if the SSH key needs a passphrase
	KeyPassphraseFunc PassphraseFunc
}

// buildAuthMethods returns SSH auth methods, trying key-based auth first.
func buildAuthMethods(auth SSHAuth) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	// Try SSH key auth from default locations
	home, err := os.UserHomeDir()
	if err == nil {
		keyPaths := []string{
			filepath.Join(home, ".ssh", "id_ed25519"),
			filepath.Join(home, ".ssh", "id_rsa"),
		}
		for _, path := range keyPaths {
			keyBytes, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			signer, err := ssh.ParsePrivateKey(keyBytes)
			if err != nil {
				// Key might be passphrase-protected
				if _, ok := err.(*ssh.PassphraseMissingError); ok && auth.KeyPassphraseFunc != nil {
					passphrase, err := auth.KeyPassphraseFunc()
					if err != nil {
						continue
					}
					signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, passphrase)
					if err != nil {
						continue
					}
				} else {
					continue
				}
			}
			methods = append(methods, ssh.PublicKeys(signer))
			break
		}
	}

	// Fall back to password auth
	if auth.Password != "" {
		methods = append(methods, ssh.Password(auth.Password))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no SSH authentication methods available")
	}
	return methods, nil
}

// RunRemote connects to the remote host via SSH and executes the embedded
// peon.sh bootstrap script. It returns the peon private key extracted from
// the script output.
func RunRemote(host, user string, auth SSHAuth) (string, error) {
	methods, err := buildAuthMethods(auth)
	if err != nil {
		return "", err
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            methods,
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
			strings.NewReader(auth.SudoPassword+"\n"),
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
