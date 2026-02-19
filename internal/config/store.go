package config

// Credential represents a stored credential entry.
type Credential struct {
	Service string
	Name    string
	Key     string
	Value   string
}

// Store abstracts over the backing storage for arnor configuration and credentials.
type Store interface {
	// Config (replaces Load/Save)
	LoadConfig() (*Config, error)
	SaveConfig(cfg *Config) error

	// Credentials (replaces os.Getenv for secrets)
	GetCredential(service, name, key string) (string, error)
	SetCredential(service, name, key, value string) error
	ListCredentials(service string) ([]Credential, error)
	DeleteCredential(service, name string) error

	// Peon keys (replaces PEON_SSH_KEY_<host> env vars)
	GetPeonKey(serverIP string) (string, error)
	SetPeonKey(serverIP, privateKey, keyPath string) error

	// Hetzner project management
	ListHetznerProjects() ([]HetznerProject, error)

	Close() error
}
