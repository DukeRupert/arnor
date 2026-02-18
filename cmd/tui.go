package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
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
	screens := map[tui.Screen]tea.Model{
		tui.ScreenServerInit: serverinit.New(),
	}
	return tui.Run(tui.ScreenServerInit, screens)
}
