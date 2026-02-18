package dns

import (
	"fmt"
	"os"

	"github.com/dukerupert/shadowfax/pkg/porkbun"
)

// PorkbunProvider adapts the shadowfax Porkbun client to the Provider interface.
type PorkbunProvider struct {
	client *porkbun.Client
}

func NewPorkbunProvider() (*PorkbunProvider, error) {
	apiKey := os.Getenv("PORKBUN_API_KEY")
	secretKey := os.Getenv("PORKBUN_SECRET_KEY")
	if apiKey == "" || secretKey == "" {
		return nil, fmt.Errorf("PORKBUN_API_KEY and PORKBUN_SECRET_KEY must be set")
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
