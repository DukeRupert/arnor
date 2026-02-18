package project

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dukerupert/annuminas/pkg/dockerhub"
	"github.com/dukerupert/arnor/internal/caddy"
	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/dns"
	"github.com/dukerupert/arnor/internal/hetzner"
	"golang.org/x/crypto/ssh"
)

// ProgressFunc is called to report step-by-step progress during setup.
type ProgressFunc func(step, total int, message string)

// SetupParams contains all inputs for project creation.
type SetupParams struct {
	ProjectName string
	Repo        string // e.g. "github.com/fireflysoftware/myclient"
	ServerName  string
	EnvName     string // "dev" or "prod"
	Domain      string
	Port    int
	PeonKey string // PEM-encoded peon SSH key; falls back to PEON_SSH_KEY env var
	OnProgress  ProgressFunc
}

// Setup runs the full project creation orchestration for a single environment.
func Setup(params SetupParams) error {
	const totalSteps = 9
	report := func(step int, message string) {
		if params.OnProgress != nil {
			params.OnProgress(step, totalSteps, message)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Step 1: Look up server IP
	report(1, "Looking up server...")
	server := cfg.FindServer(params.ServerName)
	if server == nil {
		mgr, err := hetzner.NewManager(cfg.HetznerProjects)
		if err != nil {
			return fmt.Errorf("creating Hetzner manager: %w", err)
		}
		s, err := mgr.GetServer(params.ServerName)
		if err != nil {
			return fmt.Errorf("server %q not found in config or Hetzner: %w", params.ServerName, err)
		}
		server = &config.Server{
			Name:           s.Name,
			IP:             s.PublicNet.IPv4.IP,
			HetznerProject: s.ProjectAlias,
			HetznerID:      s.ID,
		}
	}

	// Step 2: Detect DNS provider
	report(2, "Detecting DNS provider...")
	provider, err := dns.ProviderForDomain(params.Domain, cfg)
	if err != nil {
		return fmt.Errorf("detecting DNS provider for %s: %w", params.Domain, err)
	}

	// Step 3: Create DockerHub repo
	report(3, "Creating DockerHub repo...")
	dockerHubUsername := os.Getenv("DOCKERHUB_USERNAME")
	dockerHubPassword := os.Getenv("DOCKERHUB_PASSWORD")
	dockerHubToken := os.Getenv("DOCKERHUB_TOKEN")
	if dockerHubUsername == "" || dockerHubPassword == "" {
		return fmt.Errorf("DOCKERHUB_USERNAME and DOCKERHUB_PASSWORD env vars must be set")
	}
	dockerImage := dockerHubUsername + "/" + params.ProjectName
	dhClient := dockerhub.NewClient(dockerHubUsername, dockerHubPassword)
	if err := dhClient.EnsureRepo(dockerHubUsername, params.ProjectName); err != nil {
		return fmt.Errorf("creating DockerHub repo: %w", err)
	}

	// Step 4: SSH setup
	report(4, "Setting up deploy user on VPS...")
	deployUser := deployUserName(params.ProjectName, params.EnvName)
	deployPath := fmt.Sprintf("/opt/%s", deployDirName(params.ProjectName, params.EnvName))

	peonKey := params.PeonKey
	if peonKey == "" {
		peonKey = os.Getenv("PEON_SSH_KEY")
	}
	if peonKey == "" {
		return fmt.Errorf("peon SSH key not provided and PEON_SSH_KEY env var is not set")
	}

	sshResult, err := RunSetup(server.IP, deployUser, deployPath, peonKey)
	if err != nil {
		return fmt.Errorf("SSH setup: %w", err)
	}

	// Step 5: Write Caddy config
	report(5, "Writing Caddy config...")
	caddyConfig := caddy.Generate(params.Domain, params.Port)
	if err := writeCaddyConfig(server.IP, peonKey, params.Domain, caddyConfig); err != nil {
		return fmt.Errorf("writing Caddy config: %w", err)
	}

	// Step 6: Create DNS records
	report(6, "Creating DNS records...")

	// Split domain into root domain and subdomain name for the DNS API.
	// e.g. "foo.angmar.dev" → root "angmar.dev", subName "foo"
	rootDomain, err := config.RootDomain(params.Domain)
	if err != nil {
		return fmt.Errorf("resolving root domain for %s: %w", params.Domain, err)
	}
	subName := ""
	if rootDomain != params.Domain {
		subName = strings.TrimSuffix(params.Domain, "."+rootDomain)
	}

	_, err = provider.CreateRecord(rootDomain, subName, "A", server.IP, "600")
	if err != nil {
		return fmt.Errorf("creating A record: %w", err)
	}

	// Best-effort www CNAME
	wwwName := "www"
	if subName != "" {
		wwwName = "www." + subName
	}
	provider.CreateRecord(rootDomain, wwwName, "CNAME", params.Domain, "600")

	// Step 7: Set GitHub Actions secrets
	report(7, "Setting GitHub secrets...")
	prefix := strings.ToUpper(params.EnvName)
	// Prefer PAT for CI (narrower scope); fall back to password
	dockerHubCI := dockerHubToken
	if dockerHubCI == "" {
		dockerHubCI = dockerHubPassword
	}
	if err := SetEnvironmentSecrets(params.Repo, prefix, deployUser, deployPath, sshResult.DeployPublicKey, server.IP, dockerHubUsername, dockerHubCI); err != nil {
		return fmt.Errorf("setting GitHub secrets: %w", err)
	}

	// Step 8: Generate workflow files
	report(8, "Generating workflow files...")
	if err := generateWorkflowFile(params.EnvName, dockerImage); err != nil {
		return fmt.Errorf("generating workflow: %w", err)
	}

	// Step 9: Update config
	report(9, "Updating config...")
	branch := "dev"
	if params.EnvName == "prod" {
		branch = "main"
	}

	env := config.Environment{
		Domain:      params.Domain,
		DNSProvider: provider.Name(),
		Branch:      branch,
		DeployPath:  deployPath,
		DeployUser:  deployUser,
		Port:        params.Port,
	}

	existingProject := cfg.FindProject(params.ProjectName)
	if existingProject != nil {
		if existingProject.Environments == nil {
			existingProject.Environments = make(map[string]config.Environment)
		}
		existingProject.Environments[params.EnvName] = env
	} else {
		cfg.Projects = append(cfg.Projects, config.Project{
			Name:   params.ProjectName,
			Repo:   params.Repo,
			Server: params.ServerName,
			Environments: map[string]config.Environment{
				params.EnvName: env,
			},
		})
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	return nil
}

func deployUserName(project, env string) string {
	if env == "dev" {
		return project + "-dev-deploy"
	}
	return project + "-deploy"
}

func deployDirName(project, env string) string {
	if env == "dev" {
		return project + "-dev"
	}
	return project
}

func writeCaddyConfig(serverIP, peonKeyPEM, domain, caddyConfig string) error {
	signer, err := ssh.ParsePrivateKey([]byte(peonKeyPEM))
	if err != nil {
		return fmt.Errorf("parsing peon SSH key: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            "peon",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", serverIP+":22", sshConfig)
	if err != nil {
		return fmt.Errorf("SSH dial to %s: %w", serverIP, err)
	}
	defer client.Close()

	// Ensure sites directory exists
	if err := runSSHCommand(client, "sudo mkdir -p /etc/caddy/conf.d"); err != nil {
		return fmt.Errorf("creating caddy sites dir: %w", err)
	}

	// Write config via stdin to avoid shell escaping issues with echo
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("creating SSH session: %w", err)
	}
	var stderr bytes.Buffer
	session.Stdin = strings.NewReader(caddyConfig)
	session.Stderr = &stderr
	if err := session.Run(fmt.Sprintf("sudo tee /etc/caddy/conf.d/%s.caddy > /dev/null", domain)); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		session.Close()
		if errMsg != "" {
			return fmt.Errorf("writing caddy config: %s", errMsg)
		}
		return fmt.Errorf("writing caddy config: %w", err)
	}
	session.Close()

	if err := runSSHCommand(client, "sudo systemctl reload caddy"); err != nil {
		return fmt.Errorf("reloading caddy: %w", err)
	}
	return nil
}

func generateWorkflowFile(envName, dockerImage string) error {
	var content string
	var filename string
	var err error

	switch envName {
	case "dev":
		content, err = GenerateDevWorkflow(dockerImage)
		filename = "deploy-dev.yml"
	case "prod":
		content, err = GenerateProdWorkflow(dockerImage)
		filename = "deploy-prod.yml"
	default:
		return fmt.Errorf("unknown environment: %s", envName)
	}
	if err != nil {
		return err
	}

	dir := filepath.Join(".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating workflows dir: %w", err)
	}

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}
