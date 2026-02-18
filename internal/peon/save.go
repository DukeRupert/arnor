package peon

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SaveResult holds the paths and identifiers written by SavePeonKey.
type SaveResult struct {
	KeyPath string // e.g. ~/.ssh/peon_ed25519_1.2.3.4
	EnvKey  string // e.g. PEON_SSH_KEY_1_2_3_4
	EnvPath string // e.g. ~/.dotfiles/.env
}

// SavePeonKey writes the peon private key to ~/.ssh/peon_ed25519_<host> and
// upserts the path into ~/.dotfiles/.env as PEON_SSH_KEY_<host>.
func SavePeonKey(host, key string) (*SaveResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not determine home directory: %w", err)
	}

	// Strip port if present for the filename
	hostName := host
	if idx := strings.Index(host, ":"); idx != -1 {
		hostName = host[:idx]
	}

	keyFile := fmt.Sprintf("peon_ed25519_%s", hostName)
	keyPath := filepath.Join(home, ".ssh", keyFile)
	if err := os.WriteFile(keyPath, []byte(key+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("failed to write key to %s: %w", keyPath, err)
	}

	envKey := fmt.Sprintf("PEON_SSH_KEY_%s", strings.ReplaceAll(hostName, ".", "_"))
	envPath := filepath.Join(home, ".dotfiles", ".env")
	if err := upsertEnvVar(envPath, envKey, keyPath); err != nil {
		return nil, fmt.Errorf("failed to update %s: %w", envPath, err)
	}

	return &SaveResult{
		KeyPath: keyPath,
		EnvKey:  envKey,
		EnvPath: envPath,
	}, nil
}

// upsertEnvVar updates or appends a KEY=value line in the given env file.
func upsertEnvVar(path, key, value string) error {
	line := fmt.Sprintf("%s=%s", key, value)
	prefix := key + "="

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, []byte(line+"\n"), 0600)
		}
		return err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var lines []string
	replaced := false
	for scanner.Scan() {
		l := scanner.Text()
		if strings.HasPrefix(l, prefix) {
			lines = append(lines, line)
			replaced = true
		} else {
			lines = append(lines, l)
		}
	}
	if !replaced {
		lines = append(lines, line)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600)
}
