package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/hetzner"
	"github.com/dukerupert/arnor/internal/project"
	"github.com/dukerupert/arnor/tui"
	"github.com/dukerupert/arnor/tui/deploy"
	"github.com/dukerupert/arnor/tui/dockerps"
	"github.com/dukerupert/arnor/tui/menu"
	"github.com/dukerupert/arnor/tui/projectcreate"
	"github.com/dukerupert/arnor/tui/projectinspect"
	"github.com/dukerupert/arnor/tui/serverinit"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive TUI",
	RunE:  runTUI,
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

func runTUI(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if len(cfg.HetznerProjects) == 0 {
		return fmt.Errorf("no hetzner projects configured — run 'arnor config init' first")
	}

	mgr, err := hetzner.NewManager(cfg.HetznerProjects)
	if err != nil {
		return fmt.Errorf("initializing hetzner: %w", err)
	}

	servers, err := mgr.ListAllServers()
	if err != nil {
		return fmt.Errorf("fetching servers: %w", err)
	}
	if len(servers) == 0 {
		return fmt.Errorf("no servers found across hetzner projects")
	}

	repos, err := project.ListGitHubRepos()
	if err != nil {
		return fmt.Errorf("fetching github repos: %w", err)
	}

	screens := map[tui.Screen]tea.Model{
		tui.ScreenMenu:           menu.New(),
		tui.ScreenServerInit:     serverinit.New(servers),
		tui.ScreenProjectCreate:  projectcreate.New(repos, servers),
		tui.ScreenDeploy:         deploy.New(cfg.Projects),
		tui.ScreenProjectInspect: projectinspect.New(cfg.Projects),
		tui.ScreenDockerPS:       dockerps.New(servers),
	}

	factories := map[tui.Screen]tui.ScreenFactory{
		tui.ScreenServerInit: func() tea.Model {
			return serverinit.New(servers)
		},
		tui.ScreenProjectCreate: func() tea.Model {
			return projectcreate.New(repos, servers)
		},
		tui.ScreenDeploy: func() tea.Model {
			// Re-load config so newly created projects appear.
			if fresh, err := config.Load(); err == nil {
				return deploy.New(fresh.Projects)
			}
			return deploy.New(cfg.Projects)
		},
		tui.ScreenProjectInspect: func() tea.Model {
			if fresh, err := config.Load(); err == nil {
				return projectinspect.New(fresh.Projects)
			}
			return projectinspect.New(cfg.Projects)
		},
		tui.ScreenDockerPS: func() tea.Model {
			return dockerps.New(servers)
		},
	}

	return tui.Run(tui.ScreenMenu, screens, factories)
}
