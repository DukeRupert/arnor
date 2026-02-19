package config

import (
	"testing"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(:memory:): %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCredentialRoundTrip(t *testing.T) {
	s := newTestStore(t)

	// Set and get a credential.
	if err := s.SetCredential("hetzner", "prod", "api_token", "tok123"); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}

	val, err := s.GetCredential("hetzner", "prod", "api_token")
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if val != "tok123" {
		t.Errorf("got %q, want %q", val, "tok123")
	}

	// Upsert should overwrite.
	if err := s.SetCredential("hetzner", "prod", "api_token", "tok456"); err != nil {
		t.Fatalf("SetCredential (upsert): %v", err)
	}
	val, err = s.GetCredential("hetzner", "prod", "api_token")
	if err != nil {
		t.Fatalf("GetCredential after upsert: %v", err)
	}
	if val != "tok456" {
		t.Errorf("got %q, want %q", val, "tok456")
	}
}

func TestCredentialNotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetCredential("hetzner", "prod", "api_token")
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestListCredentials(t *testing.T) {
	s := newTestStore(t)

	s.SetCredential("hetzner", "prod", "api_token", "tok1")
	s.SetCredential("hetzner", "dev", "api_token", "tok2")
	s.SetCredential("porkbun", "default", "api_key", "pk1")

	creds, err := s.ListCredentials("hetzner")
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(creds) != 2 {
		t.Fatalf("got %d credentials, want 2", len(creds))
	}
}

func TestDeleteCredential(t *testing.T) {
	s := newTestStore(t)

	s.SetCredential("hetzner", "prod", "api_token", "tok1")
	s.SetCredential("hetzner", "prod", "extra", "val")

	if err := s.DeleteCredential("hetzner", "prod"); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}

	creds, _ := s.ListCredentials("hetzner")
	if len(creds) != 0 {
		t.Errorf("got %d credentials after delete, want 0", len(creds))
	}
}

func TestPeonKeyRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetPeonKey("1.2.3.4", "-----BEGIN KEY-----\nfoo\n-----END KEY-----", "/home/user/.ssh/peon_1.2.3.4"); err != nil {
		t.Fatalf("SetPeonKey: %v", err)
	}

	key, err := s.GetPeonKey("1.2.3.4")
	if err != nil {
		t.Fatalf("GetPeonKey: %v", err)
	}
	if key != "-----BEGIN KEY-----\nfoo\n-----END KEY-----" {
		t.Errorf("unexpected key: %q", key)
	}
}

func TestPeonKeyNotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetPeonKey("1.2.3.4")
	if err == nil {
		t.Fatal("expected error for missing peon key")
	}
}

func TestListHetznerProjects(t *testing.T) {
	s := newTestStore(t)

	s.SetCredential("hetzner", "prod", "api_token", "tok1")
	s.SetCredential("hetzner", "dev", "api_token", "tok2")
	s.SetCredential("porkbun", "default", "api_key", "pk1")

	projects, err := s.ListHetznerProjects()
	if err != nil {
		t.Fatalf("ListHetznerProjects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}
	// Should be ordered alphabetically.
	if projects[0].Alias != "dev" {
		t.Errorf("first project alias = %q, want %q", projects[0].Alias, "dev")
	}
	if projects[1].Alias != "prod" {
		t.Errorf("second project alias = %q, want %q", projects[1].Alias, "prod")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	s := newTestStore(t)

	// Add a Hetzner credential so HetznerProjects is populated.
	s.SetCredential("hetzner", "prod", "api_token", "tok1")

	cfg := &Config{
		Servers: []Server{
			{Name: "web1", IP: "1.2.3.4", HetznerProject: "prod", HetznerID: 42},
		},
		Projects: []Project{
			{
				Name:   "myapp",
				Repo:   "org/myapp",
				Server: "web1",
				Environments: map[string]Environment{
					"dev": {
						Domain:      "myapp.angmar.dev",
						DNSProvider: "porkbun",
						Branch:      "dev",
						DeployPath:  "/opt/myapp-dev",
						DeployUser:  "myapp-dev-deploy",
						Port:        3001,
					},
					"prod": {
						Domain:      "myapp.com",
						DNSProvider: "cloudflare",
						Branch:      "main",
						DeployPath:  "/opt/myapp",
						DeployUser:  "myapp-deploy",
						Port:        3000,
					},
				},
			},
		},
	}

	if err := s.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Verify Hetzner projects.
	if len(loaded.HetznerProjects) != 1 {
		t.Errorf("got %d hetzner projects, want 1", len(loaded.HetznerProjects))
	}

	// Verify servers.
	if len(loaded.Servers) != 1 {
		t.Fatalf("got %d servers, want 1", len(loaded.Servers))
	}
	if loaded.Servers[0].Name != "web1" {
		t.Errorf("server name = %q, want %q", loaded.Servers[0].Name, "web1")
	}
	if loaded.Servers[0].HetznerID != 42 {
		t.Errorf("server hetzner_id = %d, want 42", loaded.Servers[0].HetznerID)
	}

	// Verify projects.
	if len(loaded.Projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(loaded.Projects))
	}
	p := loaded.Projects[0]
	if p.Name != "myapp" {
		t.Errorf("project name = %q, want %q", p.Name, "myapp")
	}
	if len(p.Environments) != 2 {
		t.Fatalf("got %d environments, want 2", len(p.Environments))
	}

	dev := p.Environments["dev"]
	if dev.Port != 3001 {
		t.Errorf("dev port = %d, want 3001", dev.Port)
	}
	if dev.DeployUser != "myapp-dev-deploy" {
		t.Errorf("dev deploy_user = %q, want %q", dev.DeployUser, "myapp-dev-deploy")
	}

	prod := p.Environments["prod"]
	if prod.Domain != "myapp.com" {
		t.Errorf("prod domain = %q, want %q", prod.Domain, "myapp.com")
	}
}

func TestSaveConfigUpsert(t *testing.T) {
	s := newTestStore(t)

	cfg1 := &Config{
		Servers: []Server{{Name: "web1", IP: "1.2.3.4", HetznerProject: "prod", HetznerID: 42}},
		Projects: []Project{{
			Name: "myapp", Repo: "org/myapp", Server: "web1",
			Environments: map[string]Environment{
				"dev": {Domain: "myapp.angmar.dev", DNSProvider: "porkbun", Branch: "dev", DeployPath: "/opt/myapp-dev", DeployUser: "myapp-dev-deploy", Port: 3001},
			},
		}},
	}
	if err := s.SaveConfig(cfg1); err != nil {
		t.Fatalf("SaveConfig 1: %v", err)
	}

	// Update: change server IP, add prod environment.
	cfg2 := &Config{
		Servers: []Server{{Name: "web1", IP: "5.6.7.8", HetznerProject: "prod", HetznerID: 42}},
		Projects: []Project{{
			Name: "myapp", Repo: "org/myapp", Server: "web1",
			Environments: map[string]Environment{
				"dev":  {Domain: "myapp.angmar.dev", DNSProvider: "porkbun", Branch: "dev", DeployPath: "/opt/myapp-dev", DeployUser: "myapp-dev-deploy", Port: 3001},
				"prod": {Domain: "myapp.com", DNSProvider: "cloudflare", Branch: "main", DeployPath: "/opt/myapp", DeployUser: "myapp-deploy", Port: 3000},
			},
		}},
	}
	if err := s.SaveConfig(cfg2); err != nil {
		t.Fatalf("SaveConfig 2: %v", err)
	}

	loaded, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if loaded.Servers[0].IP != "5.6.7.8" {
		t.Errorf("server IP = %q, want %q", loaded.Servers[0].IP, "5.6.7.8")
	}
	if len(loaded.Projects[0].Environments) != 2 {
		t.Errorf("got %d environments, want 2", len(loaded.Projects[0].Environments))
	}
}

func TestEmptyConfigLoad(t *testing.T) {
	s := newTestStore(t)

	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig on empty DB: %v", err)
	}
	if len(cfg.Servers) != 0 {
		t.Errorf("got %d servers, want 0", len(cfg.Servers))
	}
	if len(cfg.Projects) != 0 {
		t.Errorf("got %d projects, want 0", len(cfg.Projects))
	}
}
