package menu

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dukerupert/arnor/tui"
)

type menuItem struct {
	label  string
	screen tui.Screen
}

var items = []menuItem{
	{"Server Init", tui.ScreenServerInit},
	{"Project Create", tui.ScreenProjectCreate},
	{"Deploy", tui.ScreenDeploy},
	{"Project Inspect", tui.ScreenProjectInspect},
}

// Model is the main menu screen.
type Model struct {
	cursor int
}

// New creates a new menu model.
func New() Model {
	return Model{}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(items)-1 {
				m.cursor++
			}
		case "enter":
			return m, func() tea.Msg {
				return tui.SwitchScreenMsg{Screen: items[m.cursor].screen}
			}
		case "q", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(tui.TitleStyle.Render("arnor"))
	b.WriteString("\n")

	for i, item := range items {
		line := fmt.Sprintf("  %s", item.label)
		if i == m.cursor {
			line = tui.CursorStyle.Render("> " + item.label)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString(tui.HelpStyle.Render("\nj/k: navigate  enter: select  q: quit"))
	return b.String()
}
