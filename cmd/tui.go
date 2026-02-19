package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dukerupert/arnor/internal/hetzner"
	"github.com/dukerupert/arnor/internal/project"
	"github.com/dukerupert/arnor/tui"
	"github.com/dukerupert/arnor/tui/deploy"
	"github.com/dukerupert/arnor/tui/dockerps"
	"github.com/dukerupert/arnor/tui/menu"
	"github.com/dukerupert/arnor/tui/projectcreate"
	"github.com/dukerupert/arnor/tui/projectinspect"
	"github.com/dukerupert/arnor/tui/serverinit"
	"github.com/dukerupert/arnor/tui/setup"
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
	// Check if we have any Hetzner credentials — if not, show setup screen.
	projects, _ := store.ListHetznerProjects()
	if len(projects) == 0 {
		screens := map[tui.Screen]tea.Model{
			tui.ScreenSetup: setup.New(store),
		}
		return tui.Run(tui.ScreenSetup, screens, nil)
	}

	cfg, err := store.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	mgr, err := hetzner.NewManager(cfg.HetznerProjects, store)
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
		tui.ScreenServerInit:     serverinit.New(servers, store),
		tui.ScreenProjectCreate:  projectcreate.New(repos, servers, store),
		tui.ScreenDeploy:         deploy.New(cfg.Projects, store),
		tui.ScreenProjectInspect: projectinspect.New(cfg.Projects),
		tui.ScreenDockerPS:       dockerps.New(servers, store),
	}

	factories := map[tui.Screen]tui.ScreenFactory{
		tui.ScreenServerInit: func() tea.Model {
			return serverinit.New(servers, store)
		},
		tui.ScreenProjectCreate: func() tea.Model {
			return projectcreate.New(repos, servers, store)
		},
		tui.ScreenDeploy: func() tea.Model {
			if fresh, err := store.LoadConfig(); err == nil {
				return deploy.New(fresh.Projects, store)
			}
			return deploy.New(cfg.Projects, store)
		},
		tui.ScreenProjectInspect: func() tea.Model {
			if fresh, err := store.LoadConfig(); err == nil {
				return projectinspect.New(fresh.Projects)
			}
			return projectinspect.New(cfg.Projects)
		},
		tui.ScreenDockerPS: func() tea.Model {
			return dockerps.New(servers, store)
		},
	}

	return tui.Run(tui.ScreenMenu, screens, factories)
}
