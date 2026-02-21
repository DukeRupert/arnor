package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dukerupert/arnor/internal/hetzner"
	"github.com/dukerupert/arnor/internal/project"
	"github.com/dukerupert/arnor/internal/service"
	"github.com/spf13/cobra"
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage Docker Compose services",
}

var serviceDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy a Docker Compose service to a server",
	Long:  `Interactive wizard to deploy a docker-compose.yml to a Hetzner server with Caddy reverse proxy and DNS.`,
	RunE:  runServiceDeploy,
}

func init() {
	serviceCmd.AddCommand(serviceDeployCmd)
	rootCmd.AddCommand(serviceCmd)
}

func runServiceDeploy(cmd *cobra.Command, args []string) error {
	scanner := bufio.NewScanner(os.Stdin)
	prompt := func(label string) string {
		fmt.Printf("%s: ", label)
		scanner.Scan()
		return strings.TrimSpace(scanner.Text())
	}

	serviceName := prompt("Service name (e.g. uptime-kuma)")
	serverName := prompt("Server name")
	domain := prompt("Domain (e.g. status.mydomain.com)")

	// Resolve server IP and peon key for port scanning
	var serverIP, peonKey string
	cfg, err := store.LoadConfig()
	if err == nil {
		srv := cfg.FindServer(serverName)
		if srv != nil {
			serverIP = srv.IP
		} else {
			mgr, mgrErr := hetzner.NewManager(cfg.HetznerProjects, store)
			if mgrErr == nil {
				if s, sErr := mgr.GetServer(serverName); sErr == nil {
					serverIP = s.PublicNet.IPv4.IP
				}
			}
		}
	}
	if serverIP != "" {
		if key, err := store.GetPeonKey(serverIP); err == nil {
			peonKey = key
		}
	}

	// Port prompt with suggestion
	var portPrompt string
	if serverIP != "" && peonKey != "" {
		usedPorts, scanErr := project.GetUsedPorts(serverIP, peonKey)
		if scanErr == nil {
			suggested := project.SuggestPort(usedPorts)
			portPrompt = fmt.Sprintf("Port [suggested: %d]", suggested)
		} else {
			portPrompt = "Port"
		}
	} else {
		portPrompt = "Port"
	}

	portStr := prompt(portPrompt)
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port: %s", portStr)
	}

	composeFile := prompt("Path to docker-compose.yml [./docker-compose.yml]")
	if composeFile == "" {
		composeFile = "./docker-compose.yml"
	}

	// Verify compose file exists
	if _, err := os.Stat(composeFile); err != nil {
		return fmt.Errorf("compose file not found: %s", composeFile)
	}

	fmt.Println()
	return service.Deploy(service.DeployParams{
		ServiceName: serviceName,
		ServerName:  serverName,
		Domain:      domain,
		Port:        port,
		ComposeFile: composeFile,
		PeonKey:     peonKey,
		Store:       store,
		OnProgress: func(step, total int, message string) {
			fmt.Printf("Step %d/%d: %s\n", step, total, message)
		},
	})
}
