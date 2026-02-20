package project

import (
	"bytes"
	"fmt"
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
	Port        int
	PeonKey     string // PEM-encoded peon SSH key
	Store       config.Store
	OnProgress  ProgressFunc
}

// Setup runs the full project creation orchestration for a single environment.
func Setup(params SetupParams) error {
	const totalSteps = 10
	report := func(step int, message string) {
		if params.OnProgress != nil {
			params.OnProgress(step, totalSteps, message)
		}
	}

	cfg, err := params.Store.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Step 1: Look up server IP
	report(1, "Looking up server...")
	server := cfg.FindServer(params.ServerName)
	if server == nil {
		mgr, err := hetzner.NewManager(cfg.HetznerProjects, params.Store)
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
	provider, err := dns.ProviderForDomain(params.Domain, cfg, params.Store)
	if err != nil {
		return fmt.Errorf("detecting DNS provider for %s: %w", params.Domain, err)
	}

	// Step 3: Create DockerHub repo
	report(3, "Creating DockerHub repo...")
	dockerHubUsername, err := params.Store.GetCredential("dockerhub", "default", "username")
	if err != nil {
		return fmt.Errorf("dockerhub username: %w", err)
	}
	dockerHubPassword, err := params.Store.GetCredential("dockerhub", "default", "password")
	if err != nil {
		return fmt.Errorf("dockerhub password: %w", err)
	}
	dockerHubToken, _ := params.Store.GetCredential("dockerhub", "default", "token")
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
		peonKey, err = params.Store.GetPeonKey(server.IP)
		if err != nil {
			return fmt.Errorf("peon key for %s: %w", server.IP, err)
		}
	}

	sshResult, err := RunSetup(server.IP, deployUser, deployPath, peonKey)
	if err != nil {
		return fmt.Errorf("SSH setup: %w", err)
	}

	// Step 5: Write docker-compose.yml
	report(5, "Writing docker-compose.yml...")
	if err := writeComposeFile(server.IP, peonKey, deployPath, deployUser, dockerImage, params.Port); err != nil {
		return fmt.Errorf("writing docker-compose.yml: %w", err)
	}

	// Step 6: Write Caddy config
	report(6, "Writing Caddy config...")
	caddyConfig := caddy.Generate(params.Domain, params.Port)
	if err := writeCaddyConfig(server.IP, peonKey, params.Domain, caddyConfig); err != nil {
		return fmt.Errorf("writing Caddy config: %w", err)
	}

	// Step 7: Create DNS records
	report(7, "Creating DNS records...")

	// Split domain into root domain and subdomain name for the DNS API.
	// e.g. "foo.angmar.dev" -> root "angmar.dev", subName "foo"
	rootDomain, err := config.RootDomain(params.Domain)
	if err != nil {
		return fmt.Errorf("resolving root domain for %s: %w", params.Domain, err)
	}
	subName := ""
	if rootDomain != params.Domain {
		subName = strings.TrimSuffix(params.Domain, "."+rootDomain)
	}

	// Remove existing A/CNAME/ALIAS records for the domain before creating new ones.
	if existing, err := provider.ListRecords(rootDomain); err == nil {
		for _, r := range existing {
			if r.Name == params.Domain && (r.Type == "A" || r.Type == "CNAME" || r.Type == "ALIAS") {
				provider.DeleteRecord(rootDomain, r.ID)
			}
		}
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

	// Step 8: Set GitHub Actions secrets
	report(8, "Setting GitHub secrets...")
	prefix := strings.ToUpper(params.EnvName)
	// Prefer PAT for CI (narrower scope); fall back to password
	dockerHubCI := dockerHubToken
	if dockerHubCI == "" {
		dockerHubCI = dockerHubPassword
	}
	if err := SetEnvironmentSecrets(params.Repo, prefix, deployUser, deployPath, sshResult.DeployPrivateKey, server.IP, dockerHubUsername, dockerHubCI, params.Port); err != nil {
		return fmt.Errorf("setting GitHub secrets: %w", err)
	}

	// Step 9: Generate workflow files
	report(9, "Generating workflow files...")
	if err := generateWorkflowFile(params.Repo, params.EnvName, dockerImage); err != nil {
		return fmt.Errorf("generating workflow: %w", err)
	}

	// Step 10: Update config
	report(10, "Updating config...")
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

	if err := params.Store.SaveConfig(cfg); err != nil {
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

	// Validate config before reloading so we get a useful error message
	validateOut, err := runSSHCommandOutput(client, "sudo caddy validate --config /etc/caddy/Caddyfile 2>&1")
	if err != nil {
		return fmt.Errorf("caddy config validation failed: %s", strings.TrimSpace(validateOut))
	}

	if err := runSSHCommand(client, "sudo systemctl reload caddy"); err != nil {
		// Grab journal output for context
		journalOut, _ := runSSHCommandOutput(client, "sudo journalctl -u caddy -n 20 --no-pager 2>&1")
		return fmt.Errorf("reloading caddy: %w\njournal output:\n%s", err, strings.TrimSpace(journalOut))
	}
	return nil
}

func writeComposeFile(serverIP, peonKeyPEM, deployPath, deployUser, dockerImage string, port int) error {
	content := fmt.Sprintf(`services:
  web:
    image: ${DOCKER_IMAGE:-%s}
    ports:
      - "${LISTEN_PORT:-%d}:80"
    restart: unless-stopped
`, dockerImage, port)

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

	composePath := deployPath + "/docker-compose.yml"

	// Write file via sudo tee
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("creating SSH session: %w", err)
	}
	var stderr bytes.Buffer
	session.Stdin = strings.NewReader(content)
	session.Stderr = &stderr
	if err := session.Run(fmt.Sprintf("sudo tee %s > /dev/null", composePath)); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		session.Close()
		if errMsg != "" {
			return fmt.Errorf("writing compose file: %s", errMsg)
		}
		return fmt.Errorf("writing compose file: %w", err)
	}
	session.Close()

	// Chown to deploy user
	if err := runSSHCommand(client, fmt.Sprintf("sudo chown %s:%s %s", deployUser, deployUser, composePath)); err != nil {
		return fmt.Errorf("chowning compose file: %w", err)
	}

	return nil
}

func generateWorkflowFile(repo, envName, dockerImage string) error {
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

	branch, err := DefaultBranch(repo)
	if err != nil {
		return fmt.Errorf("detecting default branch: %w", err)
	}

	// Remove any non-arnor workflow files before pushing ours.
	if err := DeleteStaleWorkflows(repo, branch); err != nil {
		return fmt.Errorf("cleaning stale workflows: %w", err)
	}

	path := ".github/workflows/" + filename
	commitMsg := fmt.Sprintf("Add %s deploy workflow", envName)

	return PushWorkflowFile(repo, path, content, branch, commitMsg)
}
