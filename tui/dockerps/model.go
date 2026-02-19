package dockerps

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/hetzner"
	"github.com/dukerupert/arnor/internal/project"
	"github.com/dukerupert/arnor/tui"
)

type phase int

const (
	phaseSelectServer phase = iota
	phaseLoading
	phaseDisplay
)

type dockerPsDoneMsg struct {
	containers []project.DockerContainer
	err        error
}

// Model is the BubbleTea model for the Docker PS screen.
type Model struct {
	phase phase

	servers      []hetzner.ServerWithProject
	serverCursor int
	store        config.Store

	selectedName string
	selectedIP   string
	containers   []project.DockerContainer

	spinner  spinner.Model
	viewport viewport.Model
	ready    bool // true once we've received a WindowSizeMsg
	width    int
	height   int
	err      error
}

// New creates a new Docker PS model.
func New(servers []hetzner.ServerWithProject, store config.Store) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = tui.SpinnerStyle

	return Model{
		phase:   phaseSelectServer,
		servers: servers,
		store:   store,
		spinner: s,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

// headerHeight is the number of fixed lines above the viewport (title + server + blank + section header).
const headerHeight = 5

// footerHeight is the number of fixed lines below the viewport (help bar).
const footerHeight = 2

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Capture terminal size in all phases so it's ready when we need it.
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
		if m.phase == phaseDisplay {
			m.viewport.Width = ws.Width
			m.viewport.Height = ws.Height - headerHeight - footerHeight
		}
	}

	switch m.phase {
	case phaseSelectServer:
		return m.updateSelectServer(msg)
	case phaseLoading:
		return m.updateLoading(msg)
	case phaseDisplay:
		return m.updateDisplay(msg)
	}
	return m, nil
}

func (m Model) updateSelectServer(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.serverCursor > 0 {
				m.serverCursor--
			}
		case "down", "j":
			if m.serverCursor < len(m.servers)-1 {
				m.serverCursor++
			}
		case "enter":
			if len(m.servers) == 0 {
				return m, nil
			}
			srv := m.servers[m.serverCursor]
			m.selectedName = srv.Name
			m.selectedIP = srv.PublicNet.IPv4.IP
			m.phase = phaseLoading
			return m, tea.Batch(m.spinner.Tick, m.fetchDockerPS())
		case "esc":
			return m, func() tea.Msg {
				return tui.SwitchScreenMsg{Screen: tui.ScreenMenu}
			}
		}
	}
	return m, nil
}

func (m Model) updateLoading(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case dockerPsDoneMsg:
		m.containers = msg.containers
		m.err = msg.err
		m.phase = phaseDisplay
		m.initViewport()
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) initViewport() {
	vpHeight := m.height - headerHeight - footerHeight
	if vpHeight < 1 {
		vpHeight = 20 // fallback before first WindowSizeMsg
	}
	m.viewport = viewport.New(m.width, vpHeight)
	m.viewport.SetContent(m.renderContainers())
	m.ready = true
}

func (m Model) updateDisplay(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.phase = phaseSelectServer
			m.err = nil
			m.containers = nil
			m.ready = false
			return m, nil
		case "enter":
			return m, func() tea.Msg {
				return tui.SwitchScreenMsg{Screen: tui.ScreenMenu}
			}
		case "q":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) fetchDockerPS() tea.Cmd {
	serverIP := m.selectedIP
	s := m.store
	return func() tea.Msg {
		peonKey, err := s.GetPeonKey(serverIP)
		if err != nil {
			return dockerPsDoneMsg{err: fmt.Errorf("peon key for %s: %w — run Server Init first", serverIP, err)}
		}
		containers, err := project.DockerPS(serverIP, peonKey)
		return dockerPsDoneMsg{containers: containers, err: err}
	}
}

// renderContainers builds the scrollable content string for the viewport.
func (m Model) renderContainers() string {
	var b strings.Builder

	if len(m.containers) == 0 {
		b.WriteString("  (none running)\n")
		return b.String()
	}

	for i, c := range m.containers {
		b.WriteString("  " + tui.CursorStyle.Render(c.Name) + "\n")
		b.WriteString(fmt.Sprintf("    %-10s %s\n", tui.LabelStyle.Render("Image:"), c.Image))
		b.WriteString(fmt.Sprintf("    %-10s %s\n", tui.LabelStyle.Render("Status:"), c.Status))
		if c.Ports != "" {
			b.WriteString(fmt.Sprintf("    %-10s %s\n", tui.LabelStyle.Render("Ports:"), c.Ports))
		}
		if i < len(m.containers)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// View renders the current phase.
func (m Model) View() string {
	var b strings.Builder

	b.WriteString(tui.TitleStyle.Render("Docker Containers"))
	b.WriteString("\n")

	switch m.phase {
	case phaseSelectServer:
		if len(m.servers) == 0 {
			b.WriteString("No servers found.\n")
			b.WriteString(tui.HelpStyle.Render("\nesc: back"))
			break
		}
		b.WriteString("Select a server:\n\n")
		for i, srv := range m.servers {
			line := fmt.Sprintf("  %s  %s  (%s)", srv.Name, srv.PublicNet.IPv4.IP, srv.ProjectAlias)
			if i == m.serverCursor {
				line = tui.CursorStyle.Render("> " + fmt.Sprintf("%s  %s  (%s)", srv.Name, srv.PublicNet.IPv4.IP, srv.ProjectAlias))
			}
			b.WriteString(line + "\n")
		}
		b.WriteString(tui.HelpStyle.Render("\nj/k: navigate  enter: select  esc: back"))

	case phaseLoading:
		b.WriteString(renderField("Server", fmt.Sprintf("%s (%s)", m.selectedName, m.selectedIP)))
		b.WriteString("\n")
		b.WriteString(m.spinner.View())
		b.WriteString(" Fetching containers...")

	case phaseDisplay:
		b.WriteString(renderField("Server", fmt.Sprintf("%s (%s)", m.selectedName, m.selectedIP)))
		b.WriteString("\n")

		if m.err != nil {
			b.WriteString(tui.ErrorStyle.Render("Error: "))
			b.WriteString(m.err.Error())
			b.WriteString("\n")
		} else {
			b.WriteString(tui.LabelStyle.Render(fmt.Sprintf("── Containers (%d) ───────────────", len(m.containers))))
			b.WriteString("\n")
			b.WriteString(m.viewport.View())

			scrollPct := m.viewport.ScrollPercent() * 100
			b.WriteString("\n")
			b.WriteString(tui.HelpStyle.Render(fmt.Sprintf("j/k: scroll  esc: back  enter: menu  q: quit  %.0f%%", scrollPct)))
		}
	}

	return b.String()
}

func renderField(label, value string) string {
	return tui.LabelStyle.Render(label+":") + " " + tui.ValueStyle.Render(value) + "\n"
}
