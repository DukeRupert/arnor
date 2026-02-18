package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HetznerProjects []HetznerProject `yaml:"hetzner_projects"`
	Servers         []Server         `yaml:"servers"`
	Projects        []Project        `yaml:"projects"`
}

type HetznerProject struct {
	Alias    string `yaml:"alias"`
	TokenEnv string `yaml:"token_env"`
}

type Server struct {
	Name           string `yaml:"name"`
	IP             string `yaml:"ip"`
	HetznerProject string `yaml:"hetzner_project"`
	HetznerID      int    `yaml:"hetzner_id"`
}

type Project struct {
	Name         string                 `yaml:"name"`
	Repo         string                 `yaml:"repo"`
	Server       string                 `yaml:"server"`
	Environments map[string]Environment `yaml:"environments"`
}

type Environment struct {
	Domain      string `yaml:"domain"`
	DNSProvider string `yaml:"dns_provider"`
	Branch      string `yaml:"branch"`
	DeployPath  string `yaml:"deploy_path"`
	DeployUser  string `yaml:"deploy_user"`
	Port        int    `yaml:"port"`
}

func Path() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "arnor", "config.yaml")
}

func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	dir := filepath.Dir(Path())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(Path(), data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

func (c *Config) FindServer(name string) *Server {
	for i := range c.Servers {
		if c.Servers[i].Name == name {
			return &c.Servers[i]
		}
	}
	return nil
}

func (c *Config) FindProject(name string) *Project {
	for i := range c.Projects {
		if c.Projects[i].Name == name {
			return &c.Projects[i]
		}
	}
	return nil
}

// RootDomain walks up from a full domain to find the registrable root domain
// (the one with NS records). For example, "foo.angmar.dev" returns "angmar.dev".
// If the domain itself has NS records, it is returned as-is.
func RootDomain(domain string) (string, error) {
	candidate := domain
	for {
		nss, err := net.LookupNS(candidate)
		if err == nil && len(nss) > 0 {
			return candidate, nil
		}

		idx := strings.Index(candidate, ".")
		if idx < 0 || idx == len(candidate)-1 {
			break
		}
		candidate = candidate[idx+1:]

		if !strings.Contains(candidate, ".") {
			break
		}
	}

	return "", fmt.Errorf("looking up nameservers for %s: no NS records found", domain)
}

// DetectDNSProvider inspects nameservers for a domain and returns "porkbun",
// "cloudflare", or an error if unrecognized. If the domain is a subdomain
// (e.g. project.angmar.dev), it walks up to the parent domain to find NS records.
func DetectDNSProvider(domain string) (string, error) {
	candidate := domain
	for {
		nss, err := net.LookupNS(candidate)
		if err == nil && len(nss) > 0 {
			return matchProvider(candidate, nss)
		}

		idx := strings.Index(candidate, ".")
		if idx < 0 || idx == len(candidate)-1 {
			break
		}
		candidate = candidate[idx+1:]

		if !strings.Contains(candidate, ".") {
			break
		}
	}

	return "", fmt.Errorf("looking up nameservers for %s: no NS records found", domain)
}

func matchProvider(domain string, nss []*net.NS) (string, error) {
	for _, ns := range nss {
		host := strings.ToLower(ns.Host)
		if strings.Contains(host, "porkbun") {
			return "porkbun", nil
		}
		if strings.Contains(host, "cloudflare") {
			return "cloudflare", nil
		}
	}

	var nsNames []string
	for _, ns := range nss {
		nsNames = append(nsNames, ns.Host)
	}
	return "", fmt.Errorf("unrecognized nameservers for %s: %s", domain, strings.Join(nsNames, ", "))
}
