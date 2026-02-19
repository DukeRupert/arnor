package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	fhetzner "github.com/dukerupert/fornost/pkg/hetzner"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage arnor configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive setup: add Hetzner token, discover servers, store in DB",
	RunE:  runConfigInit,
}

var configViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Print current config from DB",
	RunE:  runConfigView,
}

var configAddCmd = &cobra.Command{
	Use:   "add <service> <name> <key> <value>",
	Short: "Set a credential (e.g. arnor config add porkbun default api_key pk1_xxx)",
	Args:  cobra.ExactArgs(4),
	RunE:  runConfigAdd,
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configViewCmd)
	configCmd.AddCommand(configAddCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	scanner := bufio.NewScanner(os.Stdin)
	prompt := func(label string) string {
		fmt.Printf("%s: ", label)
		scanner.Scan()
		return strings.TrimSpace(scanner.Text())
	}

	alias := prompt("Hetzner project alias (e.g. prod, dev, default)")
	if alias == "" {
		alias = "default"
	}

	token := prompt("Hetzner API token")
	if token == "" {
		return fmt.Errorf("token is required")
	}

	// Validate the token.
	client := fhetzner.NewClient(token)
	if err := client.Ping(); err != nil {
		return fmt.Errorf("invalid Hetzner token: %w", err)
	}
	fmt.Println("Token validated.")

	// Store the credential.
	if err := store.SetCredential("hetzner", alias, "api_token", token); err != nil {
		return err
	}

	// Discover servers.
	servers, err := client.ListServers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not list servers: %v\n", err)
	} else {
		cfg, _ := store.LoadConfig()
		for _, s := range servers {
			found := false
			for _, existing := range cfg.Servers {
				if existing.Name == s.Name {
					found = true
					break
				}
			}
			if !found {
				cfg.Servers = append(cfg.Servers, struct {
					Name           string
					IP             string
					HetznerProject string
					HetznerID      int
				}{
					Name:           s.Name,
					IP:             s.PublicNet.IPv4.IP,
					HetznerProject: alias,
					HetznerID:      s.ID,
				})
			}
		}
		if err := store.SaveConfig(cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Printf("Discovered %d server(s).\n", len(servers))
	}

	fmt.Println("\nHetzner project stored. Run again to add more projects.")
	fmt.Println("Use 'arnor config add' to set Porkbun, Cloudflare, or DockerHub credentials.")
	return nil
}

func runConfigView(cmd *cobra.Command, args []string) error {
	cfg, err := store.LoadConfig()
	if err != nil {
		return err
	}

	// Hetzner projects
	projects, _ := store.ListHetznerProjects()
	if len(projects) > 0 {
		fmt.Println("Hetzner Projects:")
		for _, p := range projects {
			fmt.Printf("  - %s\n", p.Alias)
		}
		fmt.Println()
	}

	// Servers
	if len(cfg.Servers) > 0 {
		fmt.Println("Servers:")
		for _, s := range cfg.Servers {
			fmt.Printf("  - %s (%s) [%s]\n", s.Name, s.IP, s.HetznerProject)
		}
		fmt.Println()
	}

	// Projects
	if len(cfg.Projects) > 0 {
		fmt.Println("Projects:")
		for _, p := range cfg.Projects {
			fmt.Printf("  - %s (%s) on %s\n", p.Name, p.Repo, p.Server)
			for envName, env := range p.Environments {
				fmt.Printf("      [%s] %s port:%d branch:%s\n", envName, env.Domain, env.Port, env.Branch)
			}
		}
		fmt.Println()
	}

	// Credentials summary (names only, no values)
	for _, svc := range []string{"porkbun", "cloudflare", "dockerhub"} {
		creds, _ := store.ListCredentials(svc)
		if len(creds) > 0 {
			fmt.Printf("%s credentials:\n", svc)
			seen := make(map[string]bool)
			for _, c := range creds {
				key := c.Name + "/" + c.Key
				if !seen[key] {
					fmt.Printf("  - %s/%s: ***\n", c.Name, c.Key)
					seen[key] = true
				}
			}
			fmt.Println()
		}
	}

	if len(projects) == 0 && len(cfg.Servers) == 0 && len(cfg.Projects) == 0 {
		fmt.Println("No configuration found. Run 'arnor config init' to get started.")
	}

	return nil
}

func runConfigAdd(cmd *cobra.Command, args []string) error {
	service, name, key, value := args[0], args[1], args[2], args[3]
	if err := store.SetCredential(service, name, key, value); err != nil {
		return err
	}
	fmt.Printf("Stored %s/%s/%s\n", service, name, key)
	return nil
}
