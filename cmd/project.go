package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/hetzner"
	"github.com/dukerupert/arnor/internal/project"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured projects",
	RunE:  runProjectList,
}

var projectViewCmd = &cobra.Command{
	Use:   "view [name]",
	Short: "Show project details",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectView,
}

var projectCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Interactive wizard to set up a new project",
	RunE:  runProjectCreate,
}

func init() {
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectViewCmd)
	projectCmd.AddCommand(projectCreateCmd)
	rootCmd.AddCommand(projectCmd)
}

func runProjectList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if len(cfg.Projects) == 0 {
		fmt.Println("No projects configured.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tREPO\tSERVER\tDEV DOMAIN\tPROD DOMAIN")
	fmt.Fprintln(w, "────\t────\t──────\t──────────\t───────────")
	for _, p := range cfg.Projects {
		devDomain := "-"
		prodDomain := "-"
		if env, ok := p.Environments["dev"]; ok {
			devDomain = env.Domain
		}
		if env, ok := p.Environments["prod"]; ok {
			prodDomain = env.Domain
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.Repo, p.Server, devDomain, prodDomain)
	}
	return w.Flush()
}

func runProjectView(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	p := cfg.FindProject(args[0])
	if p == nil {
		return fmt.Errorf("project not found: %s", args[0])
	}

	fmt.Printf("Name:   %s\n", p.Name)
	fmt.Printf("Repo:   %s\n", p.Repo)
	fmt.Printf("Server: %s\n", p.Server)

	for envName, env := range p.Environments {
		fmt.Printf("\n[%s]\n", envName)
		fmt.Printf("  Domain:      %s\n", env.Domain)
		fmt.Printf("  DNS Provider: %s\n", env.DNSProvider)
		fmt.Printf("  Branch:      %s\n", env.Branch)
		fmt.Printf("  Deploy Path: %s\n", env.DeployPath)
		fmt.Printf("  Deploy User: %s\n", env.DeployUser)
		fmt.Printf("  Port:        %d\n", env.Port)
	}
	return nil
}

func runProjectCreate(cmd *cobra.Command, args []string) error {
	scanner := bufio.NewScanner(os.Stdin)
	prompt := func(label string) string {
		fmt.Printf("%s: ", label)
		scanner.Scan()
		return strings.TrimSpace(scanner.Text())
	}

	projectName := prompt("Project name")
	repo := prompt("GitHub repo (e.g. github.com/org/repo)")
	serverName := prompt("Server name")

	envChoice := prompt("Environment (dev/prod/both)")

	var environments []string
	switch envChoice {
	case "both":
		environments = []string{"dev", "prod"}
	case "dev", "prod":
		environments = []string{envChoice}
	default:
		return fmt.Errorf("invalid environment: %s (must be dev, prod, or both)", envChoice)
	}

	// Resolve server IP and peon key for port scanning
	var serverIP, peonKey string
	cfg, err := config.Load()
	if err == nil {
		srv := cfg.FindServer(serverName)
		if srv != nil {
			serverIP = srv.IP
		} else {
			mgr, mgrErr := hetzner.NewManager(cfg.HetznerProjects)
			if mgrErr == nil {
				if s, sErr := mgr.GetServer(serverName); sErr == nil {
					serverIP = s.PublicNet.IPv4.IP
				}
			}
		}
	}
	if serverIP != "" {
		envKey := "PEON_SSH_KEY_" + strings.ReplaceAll(serverIP, ".", "_")
		keyPath := os.Getenv(envKey)
		if keyPath != "" {
			if data, readErr := os.ReadFile(keyPath); readErr == nil {
				peonKey = strings.TrimSpace(string(data))
			}
		}
	}

	for _, envName := range environments {
		fmt.Printf("\n--- %s environment ---\n", strings.ToUpper(envName))

		defaultDomain := ""
		if envName == "dev" {
			defaultDomain = projectName + ".angmar.dev"
			fmt.Printf("Domain [%s]: ", defaultDomain)
		} else {
			fmt.Print("Domain: ")
		}
		scanner.Scan()
		domain := strings.TrimSpace(scanner.Text())
		if domain == "" {
			domain = defaultDomain
		}
		if domain == "" {
			return fmt.Errorf("domain is required for %s environment", envName)
		}

		// Scan used ports and suggest next available
		var portPrompt string
		if serverIP != "" && peonKey != "" {
			usedPorts, scanErr := project.GetUsedPorts(serverIP, peonKey)
			if scanErr == nil {
				suggested := project.SuggestPort(usedPorts)
				portPrompt = fmt.Sprintf("Port for %s [suggested: %d]", envName, suggested)
			} else {
				portPrompt = fmt.Sprintf("Port for %s", envName)
			}
		} else {
			portPrompt = fmt.Sprintf("Port for %s", envName)
		}

		portStr := prompt(portPrompt)
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port: %s", portStr)
		}

		fmt.Println()
		if err := project.Setup(project.SetupParams{
			ProjectName: projectName,
			Repo:        repo,
			ServerName:  serverName,
			EnvName:     envName,
			Domain:      domain,
			Port:        port,
			PeonKey:     peonKey,
			OnProgress: func(step, total int, message string) {
				fmt.Printf("Step %d/%d: %s\n", step, total, message)
			},
		}); err != nil {
			return fmt.Errorf("%s setup failed: %w", envName, err)
		}
	}

	return nil
}
