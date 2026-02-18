package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/dukerupert/arnor/internal/config"
	fhetzner "github.com/dukerupert/fornost/pkg/hetzner"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage arnor configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Auto-generate config by querying provider APIs",
	RunE:  runConfigInit,
}

var configViewCmd = &cobra.Command{
	Use:   "view",
	Short: "Print current config",
	RunE:  runConfigView,
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configViewCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	cfg := &config.Config{}

	// Discover Hetzner projects from known env var patterns
	hetznerEnvVars := discoverHetznerTokens()
	for alias, envVar := range hetznerEnvVars {
		token := os.Getenv(envVar)
		if token == "" {
			fmt.Fprintf(os.Stderr, "Skipping Hetzner project %q: %s is empty\n", alias, envVar)
			continue
		}

		cfg.HetznerProjects = append(cfg.HetznerProjects, config.HetznerProject{
			Alias:    alias,
			TokenEnv: envVar,
		})

		// Fetch servers for this project
		client := fhetzner.NewClient(token)
		servers, err := client.ListServers()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not list servers for %s: %v\n", alias, err)
			continue
		}

		for _, s := range servers {
			cfg.Servers = append(cfg.Servers, config.Server{
				Name:           s.Name,
				IP:             s.PublicNet.IPv4.IP,
				HetznerProject: alias,
				HetznerID:      s.ID,
			})
		}
	}

	if err := config.Save(cfg); err != nil {
		return err
	}

	fmt.Printf("Config written to %s\n", config.Path())
	fmt.Printf("  Hetzner projects: %d\n", len(cfg.HetznerProjects))
	fmt.Printf("  Servers: %d\n", len(cfg.Servers))
	return nil
}

func runConfigView(cmd *cobra.Command, args []string) error {
	data, err := os.ReadFile(config.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no config found — run 'arnor config init' first")
		}
		return err
	}

	// Re-marshal for consistent formatting
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		// Fall back to printing raw
		fmt.Print(string(data))
		return nil
	}
	out, _ := yaml.Marshal(raw)
	fmt.Print(string(out))
	return nil
}

// discoverHetznerTokens looks for HETZNER_API_TOKEN_* env vars and returns
// a map of alias -> env var name. Also checks the single HETZNER_API_TOKEN.
func discoverHetznerTokens() map[string]string {
	tokens := make(map[string]string)

	// Check for the common multi-project pattern
	for _, suffix := range []string{"PROD", "DEV", "STAGING"} {
		envVar := "HETZNER_API_TOKEN_" + suffix
		if os.Getenv(envVar) != "" {
			alias := strings.ToLower(suffix)
			tokens[alias] = envVar
		}
	}

	// Fall back to single token
	if len(tokens) == 0 && os.Getenv("HETZNER_API_TOKEN") != "" {
		tokens["default"] = "HETZNER_API_TOKEN"
	}

	return tokens
}
