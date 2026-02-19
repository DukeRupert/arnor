package peon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dukerupert/arnor/internal/config"
)

// SaveResult holds the paths and identifiers written by SavePeonKey.
type SaveResult struct {
	KeyPath string // e.g. ~/.ssh/peon_ed25519_1.2.3.4
}

// SavePeonKey writes the peon private key to ~/.ssh/peon_ed25519_<host> and
// stores the key content + path in the Store.
func SavePeonKey(host, key string, store config.Store) (*SaveResult, error) {
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

	// Store key in the database.
	if err := store.SetPeonKey(hostName, key, keyPath); err != nil {
		return nil, fmt.Errorf("failed to store peon key: %w", err)
	}

	return &SaveResult{
		KeyPath: keyPath,
	}, nil
}
