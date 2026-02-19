package dns

import (
	"fmt"
	"strconv"

	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/gwaihir/pkg/cloudflare"
)

// CloudflareProvider adapts the gwaihir Cloudflare client to the Provider interface.
type CloudflareProvider struct {
	client    *cloudflare.Client
	accountID string
	zoneIDs   map[string]string // domain -> zoneID cache
}

func NewCloudflareProvider(store config.Store) (*CloudflareProvider, error) {
	accountID, err := store.GetCredential("cloudflare", "default", "account_id")
	if err != nil {
		return nil, fmt.Errorf("cloudflare account_id: %w", err)
	}
	token, err := store.GetCredential("cloudflare", "default", "api_token")
	if err != nil {
		return nil, fmt.Errorf("cloudflare api_token: %w", err)
	}
	return &CloudflareProvider{
		client:    cloudflare.NewClient(token),
		accountID: accountID,
		zoneIDs:   make(map[string]string),
	}, nil
}

func (c *CloudflareProvider) Name() string { return "cloudflare" }

func (c *CloudflareProvider) getZoneID(domain string) (string, error) {
	if id, ok := c.zoneIDs[domain]; ok {
		return id, nil
	}
	id, err := c.client.GetZoneID(domain)
	if err != nil {
		return "", err
	}
	c.zoneIDs[domain] = id
	return id, nil
}

func (c *CloudflareProvider) CreateRecord(domain, name, recordType, content, ttl string) (string, error) {
	zoneID, err := c.getZoneID(domain)
	if err != nil {
		return "", err
	}

	// Cloudflare expects FQDN for the name field. Porkbun uses just the subdomain.
	fqdn := name
	if name == "" {
		fqdn = domain
	} else if name != domain && !isSubdomainOf(name, domain) {
		fqdn = name + "." + domain
	}

	ttlInt := 1 // Cloudflare automatic
	if ttl != "" {
		if v, err := strconv.Atoi(ttl); err == nil {
			ttlInt = v
		}
	}

	record := cloudflare.DNSRecord{
		Type:    recordType,
		Name:    fqdn,
		Content: content,
		TTL:     ttlInt,
	}

	result, err := c.client.CreateRecord(zoneID, record)
	if err != nil {
		return "", err
	}
	return result.ID, nil
}

func (c *CloudflareProvider) DeleteRecord(domain, id string) error {
	zoneID, err := c.getZoneID(domain)
	if err != nil {
		return err
	}
	return c.client.DeleteRecord(zoneID, id)
}

func (c *CloudflareProvider) ListRecords(domain string) ([]DNSRecord, error) {
	zoneID, err := c.getZoneID(domain)
	if err != nil {
		return nil, err
	}

	records, err := c.client.ListRecords(zoneID)
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
			TTL:     strconv.Itoa(r.TTL),
		}
	}
	return out, nil
}

// isSubdomainOf checks if name is already a subdomain of domain (e.g. "www.example.com" of "example.com").
func isSubdomainOf(name, domain string) bool {
	return len(name) > len(domain) && name[len(name)-len(domain)-1:] == "."+domain
}
