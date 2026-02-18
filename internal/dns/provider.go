package dns

import (
	"fmt"

	"github.com/dukerupert/arnor/internal/config"
)

// DNSRecord is the unified record type used across providers.
type DNSRecord struct {
	ID      string
	Name    string
	Type    string
	Content string
	TTL     string
}

// Provider is the common interface for DNS operations.
type Provider interface {
	CreateRecord(domain, name, recordType, content, ttl string) (string, error)
	DeleteRecord(domain, id string) error
	ListRecords(domain string) ([]DNSRecord, error)
	Name() string
}

// NewProvider creates a DNS provider by name. Credentials are read from
// environment variables that should already be loaded via godotenv.
func NewProvider(providerName string) (Provider, error) {
	switch providerName {
	case "porkbun":
		return NewPorkbunProvider()
	case "cloudflare":
		return NewCloudflareProvider()
	default:
		return nil, fmt.Errorf("unknown DNS provider: %s", providerName)
	}
}

// ProviderForDomain determines the DNS provider for a domain. It checks the
// config first, then falls back to NS-based detection.
func ProviderForDomain(domain string, cfg *config.Config) (Provider, error) {
	// Check config for known projects with this domain
	if cfg != nil {
		for _, p := range cfg.Projects {
			for _, env := range p.Environments {
				if env.Domain == domain && env.DNSProvider != "" {
					return NewProvider(env.DNSProvider)
				}
			}
		}
	}

	// Fall back to nameserver detection
	providerName, err := config.DetectDNSProvider(domain)
	if err != nil {
		return nil, err
	}
	return NewProvider(providerName)
}
