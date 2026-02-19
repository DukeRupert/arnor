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

// NewProvider creates a DNS provider by name. Credentials are resolved from the Store.
func NewProvider(providerName string, store config.Store) (Provider, error) {
	switch providerName {
	case "porkbun":
		return NewPorkbunProvider(store)
	case "cloudflare":
		return NewCloudflareProvider(store)
	default:
		return nil, fmt.Errorf("unknown DNS provider: %s", providerName)
	}
}

// ProviderForDomain determines the DNS provider for a domain. It checks the
// config first, then falls back to NS-based detection.
func ProviderForDomain(domain string, cfg *config.Config, store config.Store) (Provider, error) {
	// Check config for known projects with this domain
	if cfg != nil {
		for _, p := range cfg.Projects {
			for _, env := range p.Environments {
				if env.Domain == domain && env.DNSProvider != "" {
					return NewProvider(env.DNSProvider, store)
				}
			}
		}
	}

	// Fall back to nameserver detection
	providerName, err := config.DetectDNSProvider(domain)
	if err != nil {
		return nil, err
	}
	return NewProvider(providerName, store)
}
