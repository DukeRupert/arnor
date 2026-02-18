package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/hetzner"
	"github.com/dukerupert/arnor/tui"
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

	screens := map[tui.Screen]tea.Model{
		tui.ScreenServerInit: serverinit.New(servers),
	}
	return tui.Run(tui.ScreenServerInit, screens)
}
