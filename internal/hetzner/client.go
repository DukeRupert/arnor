package hetzner

import (
	"fmt"

	"github.com/dukerupert/arnor/internal/config"
	fhetzner "github.com/dukerupert/fornost/pkg/hetzner"
)

// ServerWithProject embeds a fornost Server with the Hetzner project alias.
type ServerWithProject struct {
	fhetzner.Server
	ProjectAlias string
}

// SSHKeyWithProject embeds a fornost SSHKey with the Hetzner project alias.
type SSHKeyWithProject struct {
	fhetzner.SSHKey
	ProjectAlias string
}

// Manager holds clients for multiple Hetzner projects.
type Manager struct {
	clients map[string]*fhetzner.Client // alias -> client
}

// NewManager creates a Manager from config, resolving each project's token
// from the Store.
func NewManager(projects []config.HetznerProject, store config.Store) (*Manager, error) {
	m := &Manager{clients: make(map[string]*fhetzner.Client)}
	for _, p := range projects {
		token, err := store.GetCredential("hetzner", p.Alias, "api_token")
		if err != nil {
			return nil, fmt.Errorf("credential for Hetzner project %q: %w", p.Alias, err)
		}
		m.clients[p.Alias] = fhetzner.NewClient(token)
	}
	return m, nil
}

// Client returns the raw fornost client for a specific project alias.
func (m *Manager) Client(alias string) (*fhetzner.Client, error) {
	c, ok := m.clients[alias]
	if !ok {
		return nil, fmt.Errorf("unknown Hetzner project: %s", alias)
	}
	return c, nil
}

// ListAllServers aggregates servers across all Hetzner projects.
func (m *Manager) ListAllServers() ([]ServerWithProject, error) {
	var all []ServerWithProject
	for alias, client := range m.clients {
		servers, err := client.ListServers()
		if err != nil {
			return nil, fmt.Errorf("listing servers for %s: %w", alias, err)
		}
		for _, s := range servers {
			all = append(all, ServerWithProject{Server: s, ProjectAlias: alias})
		}
	}
	return all, nil
}

// GetServer finds a server by name across all projects.
func (m *Manager) GetServer(name string) (*ServerWithProject, error) {
	for alias, client := range m.clients {
		s, err := client.GetServer(name)
		if err == nil {
			return &ServerWithProject{Server: *s, ProjectAlias: alias}, nil
		}
	}
	return nil, fmt.Errorf("server not found: %s", name)
}

// ListAllSSHKeys aggregates SSH keys across all Hetzner projects.
func (m *Manager) ListAllSSHKeys() ([]SSHKeyWithProject, error) {
	var all []SSHKeyWithProject
	for alias, client := range m.clients {
		keys, err := client.ListSSHKeys()
		if err != nil {
			return nil, fmt.Errorf("listing SSH keys for %s: %w", alias, err)
		}
		for _, k := range keys {
			all = append(all, SSHKeyWithProject{SSHKey: k, ProjectAlias: alias})
		}
	}
	return all, nil
}
