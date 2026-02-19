package config

import (
	"fmt"
	"net"
	"strings"
)

type Config struct {
	HetznerProjects []HetznerProject
	Servers         []Server
	Projects        []Project
}

type HetznerProject struct {
	Alias string
}

type Server struct {
	Name           string
	IP             string
	HetznerProject string
	HetznerID      int
}

type Project struct {
	Name         string
	Repo         string
	Server       string
	Environments map[string]Environment
}

type Environment struct {
	Domain      string
	DNSProvider string
	Branch      string
	DeployPath  string
	DeployUser  string
	Port        int
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
