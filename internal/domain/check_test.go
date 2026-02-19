package domain

import (
	"testing"

	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/dns"
)

func TestLookupContext_Found(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.Server{
			{Name: "my-vps", IP: "5.78.89.141"},
		},
		Projects: []config.Project{
			{
				Name:   "myclient",
				Server: "my-vps",
				Environments: map[string]config.Environment{
					"prod": {Domain: "myclient.com", Port: 3000},
					"dev":  {Domain: "myclient.angmar.dev", Port: 3001},
				},
			},
		},
	}

	ctx := LookupContext(cfg, "myclient.com")
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.ProjectName != "myclient" {
		t.Errorf("project name = %q, want %q", ctx.ProjectName, "myclient")
	}
	if ctx.EnvName != "prod" {
		t.Errorf("env name = %q, want %q", ctx.EnvName, "prod")
	}
	if ctx.ServerName != "my-vps" {
		t.Errorf("server name = %q, want %q", ctx.ServerName, "my-vps")
	}
	if ctx.ExpectedIP != "5.78.89.141" {
		t.Errorf("expected IP = %q, want %q", ctx.ExpectedIP, "5.78.89.141")
	}
	if ctx.Port != 3000 {
		t.Errorf("port = %d, want %d", ctx.Port, 3000)
	}
}

func TestLookupContext_NotFound(t *testing.T) {
	cfg := &config.Config{
		Projects: []config.Project{
			{
				Name: "myclient",
				Environments: map[string]config.Environment{
					"prod": {Domain: "myclient.com"},
				},
			},
		},
	}

	ctx := LookupContext(cfg, "unknown.com")
	if ctx != nil {
		t.Errorf("expected nil context for unknown domain, got %+v", ctx)
	}
}

func TestLookupContext_NilConfig(t *testing.T) {
	ctx := LookupContext(nil, "example.com")
	if ctx != nil {
		t.Errorf("expected nil context for nil config, got %+v", ctx)
	}
}

func TestLookupContext_ServerNotFound(t *testing.T) {
	cfg := &config.Config{
		Projects: []config.Project{
			{
				Name:   "myclient",
				Server: "nonexistent",
				Environments: map[string]config.Environment{
					"prod": {Domain: "myclient.com", Port: 3000},
				},
			},
		},
	}

	ctx := LookupContext(cfg, "myclient.com")
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.ExpectedIP != "" {
		t.Errorf("expected empty IP when server not found, got %q", ctx.ExpectedIP)
	}
}

func TestComputeSummary(t *testing.T) {
	tests := []struct {
		name   string
		result CheckResult
		want   CheckStatus
	}{
		{
			name: "resolution fail",
			result: CheckResult{
				Resolution: ResolutionResult{Status: StatusFail},
				Context:    &DomainContext{ExpectedIP: "1.2.3.4"},
			},
			want: StatusFail,
		},
		{
			name: "no context (unknown domain)",
			result: CheckResult{
				Resolution: ResolutionResult{Status: StatusSkip, ResolvedIPs: []string{"1.2.3.4"}},
				Context:    nil,
			},
			want: StatusWarn,
		},
		{
			name: "resolution pass with context",
			result: CheckResult{
				Resolution: ResolutionResult{Status: StatusPass, ResolvedIPs: []string{"1.2.3.4"}, ExpectedIP: "1.2.3.4"},
				Context:    &DomainContext{ExpectedIP: "1.2.3.4"},
			},
			want: StatusPass,
		},
		{
			name: "resolution skip with context (no expected IP)",
			result: CheckResult{
				Resolution: ResolutionResult{Status: StatusSkip, ResolvedIPs: []string{"1.2.3.4"}},
				Context:    &DomainContext{},
			},
			want: StatusWarn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeSummary(&tt.result)
			if got != tt.want {
				t.Errorf("computeSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFilterRecords(t *testing.T) {
	domain := "myclient.com"

	records := []dns.DNSRecord{
		{ID: "1", Type: "A", Name: "myclient.com", Content: "5.78.89.141", TTL: "600"},
		{ID: "2", Type: "CNAME", Name: "www.myclient.com", Content: "myclient.com", TTL: "600"},
		{ID: "3", Type: "TXT", Name: "myclient.com", Content: "v=spf1 ...", TTL: "600"},
		{ID: "4", Type: "MX", Name: "myclient.com", Content: "mail.example.com", TTL: "600"},
		{ID: "5", Type: "A", Name: "api.myclient.com", Content: "5.78.89.142", TTL: "600"},
	}

	filtered := filterRecords(records, domain)

	// Should include: A for myclient.com, CNAME for www, A for api (has suffix .myclient.com)
	// Should exclude: TXT, MX
	if len(filtered) != 3 {
		t.Errorf("expected 3 filtered records, got %d", len(filtered))
		for _, r := range filtered {
			t.Logf("  %s %s %s", r.Type, r.Name, r.Content)
		}
	}

	// Verify TXT and MX are excluded
	for _, r := range filtered {
		if r.Type == "TXT" || r.Type == "MX" {
			t.Errorf("unexpected record type %s in filtered results", r.Type)
		}
	}
}

func TestFilterRecords_Porkbun(t *testing.T) {
	// Porkbun uses short names
	domain := "myclient.com"

	records := []dns.DNSRecord{
		{ID: "1", Type: "A", Name: "", Content: "5.78.89.141", TTL: "600"},
		{ID: "2", Type: "CNAME", Name: "www", Content: "myclient.com", TTL: "600"},
		{ID: "3", Type: "TXT", Name: "", Content: "v=spf1 ...", TTL: "600"},
	}

	filtered := filterRecords(records, domain)

	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered records (Porkbun short names), got %d", len(filtered))
		for _, r := range filtered {
			t.Logf("  %s %q %s", r.Type, r.Name, r.Content)
		}
	}
}
