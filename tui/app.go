package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Screen identifies a TUI screen.
type Screen int

const (
	ScreenServerInit Screen = iota
)

// SwitchScreenMsg tells the app to switch to a different screen.
type SwitchScreenMsg struct {
	Screen Screen
}

// app is the root model that routes between screens.
type app struct {
	screens map[Screen]tea.Model
	current Screen
}

func (a app) Init() tea.Cmd {
	return a.screens[a.current].Init()
}

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	case SwitchScreenMsg:
		a.current = msg.Screen
		return a, a.screens[a.current].Init()
	}

	updated, cmd := a.screens[a.current].Update(msg)
	a.screens[a.current] = updated
	return a, cmd
}

func (a app) View() string {
	return a.screens[a.current].View()
}

// Run starts the TUI with the given initial screen and screen map.
func Run(initial Screen, screens map[Screen]tea.Model) error {
	if _, ok := screens[initial]; !ok {
		return fmt.Errorf("initial screen %d not found in screen map", initial)
	}
	p := tea.NewProgram(app{
		screens: screens,
		current: initial,
	}, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
