package dns

import (
	"fmt"

	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/shadowfax/pkg/porkbun"
)

// PorkbunProvider adapts the shadowfax Porkbun client to the Provider interface.
type PorkbunProvider struct {
	client *porkbun.Client
}

func NewPorkbunProvider(store config.Store) (*PorkbunProvider, error) {
	apiKey, err := store.GetCredential("porkbun", "default", "api_key")
	if err != nil {
		return nil, fmt.Errorf("porkbun api_key: %w", err)
	}
	secretKey, err := store.GetCredential("porkbun", "default", "secret_key")
	if err != nil {
		return nil, fmt.Errorf("porkbun secret_key: %w", err)
	}
	return &PorkbunProvider{client: porkbun.NewClient(apiKey, secretKey)}, nil
}

func (p *PorkbunProvider) Name() string { return "porkbun" }

func (p *PorkbunProvider) CreateRecord(domain, name, recordType, content, ttl string) (string, error) {
	return p.client.CreateRecord(domain, name, recordType, content, ttl)
}

func (p *PorkbunProvider) DeleteRecord(domain, id string) error {
	return p.client.DeleteRecord(domain, id)
}

func (p *PorkbunProvider) ListRecords(domain string) ([]DNSRecord, error) {
	records, err := p.client.ListRecords(domain)
	if err != nil {
		return nil, err
	}

	out := make([]DNSRecord, len(records))
	for i, r := range records {
		out[i] = DNSRecord{
			ID:      r.ID,
			Name:    r.Name,
			Type:    r.Type,
			Content: r.Content,
			TTL:     r.TTL,
		}
	}
	return out, nil
}
