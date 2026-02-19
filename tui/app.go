package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Screen identifies a TUI screen.
type Screen int

const (
	ScreenMenu Screen = iota
	ScreenServerInit
	ScreenProjectCreate
	ScreenDeploy
	ScreenProjectInspect
)

// SwitchScreenMsg tells the app to switch to a different screen.
type SwitchScreenMsg struct {
	Screen Screen
}

// ScreenFactory creates a fresh model for a screen (used on re-entry).
type ScreenFactory func() tea.Model

// app is the root model that routes between screens.
type app struct {
	screens   map[Screen]tea.Model
	factories map[Screen]ScreenFactory
	current   Screen
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
		if factory, ok := a.factories[msg.Screen]; ok {
			a.screens[msg.Screen] = factory()
		}
		return a, a.screens[a.current].Init()
	}

	updated, cmd := a.screens[a.current].Update(msg)
	a.screens[a.current] = updated
	return a, cmd
}

func (a app) View() string {
	return a.screens[a.current].View()
}

// Run starts the TUI with the given initial screen, screen map, and optional factories.
func Run(initial Screen, screens map[Screen]tea.Model, factories map[Screen]ScreenFactory) error {
	if _, ok := screens[initial]; !ok {
		return fmt.Errorf("initial screen %d not found in screen map", initial)
	}
	p := tea.NewProgram(app{
		screens:   screens,
		factories: factories,
		current:   initial,
	}, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
