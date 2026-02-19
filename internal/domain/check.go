package domain

import (
	"fmt"
	"net"
	"strings"

	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/dns"
)

// CheckStatus represents the outcome of a check.
type CheckStatus string

const (
	StatusPass CheckStatus = "PASS"
	StatusFail CheckStatus = "FAIL"
	StatusWarn CheckStatus = "WARN"
	StatusSkip CheckStatus = "SKIP"
)

// DomainContext holds project/environment metadata for a known domain.
type DomainContext struct {
	ProjectName string
	EnvName     string
	ServerName  string
	ExpectedIP  string
	Port        int
}

// ResolutionResult holds DNS resolution details.
type ResolutionResult struct {
	ResolvedIPs []string
	ExpectedIP  string
	Status      CheckStatus
	Error       string
}

// CheckResult is the full structured result of a domain check.
type CheckResult struct {
	Domain       string
	RootDomain   string
	ProviderName string
	Context      *DomainContext
	Resolution   ResolutionResult
	Records      []dns.DNSRecord
	RecordsError string
	Summary      CheckStatus
}

// LookupContext searches all project environments for a matching domain
// and returns the associated metadata. Returns nil if not found.
func LookupContext(cfg *config.Config, domain string) *DomainContext {
	if cfg == nil {
		return nil
	}
	for _, p := range cfg.Projects {
		for envName, env := range p.Environments {
			if env.Domain == domain {
				ctx := &DomainContext{
					ProjectName: p.Name,
					EnvName:     envName,
					ServerName:  p.Server,
					Port:        env.Port,
				}
				if srv := cfg.FindServer(p.Server); srv != nil {
					ctx.ExpectedIP = srv.IP
				}
				return ctx
			}
		}
	}
	return nil
}

// Check performs a full domain health check.
func Check(cfg *config.Config, domain string) (*CheckResult, error) {
	rootDomain, err := config.RootDomain(domain)
	if err != nil {
		return nil, fmt.Errorf("resolving root domain: %w", err)
	}

	result := &CheckResult{
		Domain:     domain,
		RootDomain: rootDomain,
	}

	// Look up project context
	result.Context = LookupContext(cfg, domain)

	// Resolve DNS provider
	provider, providerErr := dns.ProviderForDomain(domain, cfg)
	if providerErr == nil {
		result.ProviderName = provider.Name()
	}

	// DNS resolution
	result.Resolution = resolve(domain, result.Context)

	// Fetch provider records
	if provider != nil {
		records, err := provider.ListRecords(rootDomain)
		if err != nil {
			result.RecordsError = err.Error()
		} else {
			result.Records = filterRecords(records, domain)
		}
	}

	result.Summary = computeSummary(result)
	return result, nil
}

// resolve performs DNS A record lookup and compares against expected IP.
func resolve(domain string, ctx *DomainContext) ResolutionResult {
	r := ResolutionResult{Status: StatusSkip}

	ips, err := net.LookupHost(domain)
	if err != nil {
		r.Status = StatusFail
		r.Error = err.Error()
		return r
	}
	r.ResolvedIPs = ips

	if ctx == nil || ctx.ExpectedIP == "" {
		r.Status = StatusSkip
		return r
	}

	r.ExpectedIP = ctx.ExpectedIP
	for _, ip := range ips {
		if ip == ctx.ExpectedIP {
			r.Status = StatusPass
			return r
		}
	}

	r.Status = StatusFail
	return r
}

// filterRecords keeps only A and CNAME records relevant to the domain.
func filterRecords(records []dns.DNSRecord, domain string) []dns.DNSRecord {
	var filtered []dns.DNSRecord
	for _, r := range records {
		if r.Type != "A" && r.Type != "CNAME" {
			continue
		}
		name := strings.TrimSuffix(r.Name, ".")
		// Match exact domain or www subdomain
		if name == domain || name == "www."+domain ||
			// Porkbun uses short names (e.g. "" for root, "www" for www)
			name == "" || name == "www" ||
			// Cloudflare uses FQDN
			strings.HasSuffix(name, "."+domain) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// computeSummary derives the overall status from individual check results.
func computeSummary(r *CheckResult) CheckStatus {
	if r.Resolution.Status == StatusFail {
		return StatusFail
	}
	if r.Context == nil {
		return StatusWarn
	}
	if r.Resolution.Status == StatusPass {
		return StatusPass
	}
	return StatusWarn
}
