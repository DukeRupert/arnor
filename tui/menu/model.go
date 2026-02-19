package menu

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dukerupert/arnor/tui"
)

type menuItem struct {
	label   string
	screen  tui.Screen
	heading string // non-empty = section header rendered above this item
}

var items = []menuItem{
	{label: "Init", screen: tui.ScreenServerInit, heading: "Servers"},
	{label: "Containers", screen: tui.ScreenDockerPS},
	{label: "Create", screen: tui.ScreenProjectCreate, heading: "Projects"},
	{label: "Deploy", screen: tui.ScreenDeploy},
	{label: "Inspect", screen: tui.ScreenProjectInspect},
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
		if item.heading != "" {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(tui.LabelStyle.Render(item.heading) + "\n")
		}
		line := fmt.Sprintf("    %s", item.label)
		if i == m.cursor {
			line = tui.CursorStyle.Render("  > " + item.label)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString(tui.HelpStyle.Render("\nj/k: navigate  enter: select  q: quit"))
	return b.String()
}
