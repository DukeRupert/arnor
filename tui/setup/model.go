package setup

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dukerupert/arnor/internal/config"
	fhetzner "github.com/dukerupert/fornost/pkg/hetzner"
	"github.com/dukerupert/arnor/tui"
)

type phase int

const (
	phaseWelcome phase = iota
	phaseToken
	phaseAlias
	phaseValidating
	phaseDone
)

type validateDoneMsg struct {
	servers []fhetzner.Server
	err     error
}

// Model is the BubbleTea model for the first-run onboarding screen.
type Model struct {
	phase phase
	store config.Store

	tokenInput textinput.Model
	aliasInput textinput.Model
	spinner    spinner.Model

	token   string
	alias   string
	servers []fhetzner.Server
	err     error
}

// New creates a new setup model.
func New(store config.Store) Model {
	ti := textinput.New()
	ti.Placeholder = "Hetzner API token"
	ti.EchoMode = textinput.EchoPassword
	ti.CharLimit = 128

	ai := textinput.New()
	ai.Placeholder = "default"
	ai.SetValue("default")
	ai.CharLimit = 32

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = tui.SpinnerStyle

	return Model{
		phase:      phaseWelcome,
		store:      store,
		tokenInput: ti,
		aliasInput: ai,
		spinner:    s,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseWelcome:
		return m.updateWelcome(msg)
	case phaseAlias:
		return m.updateAlias(msg)
	case phaseToken:
		return m.updateToken(msg)
	case phaseValidating:
		return m.updateValidating(msg)
	case phaseDone:
		return m.updateDone(msg)
	}
	return m, nil
}

func (m Model) updateWelcome(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			m.phase = phaseAlias
			m.aliasInput.Focus()
			return m, textinput.Blink
		case "q", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) updateAlias(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			val := strings.TrimSpace(m.aliasInput.Value())
			if val == "" {
				val = "default"
			}
			m.alias = val
			m.phase = phaseToken
			m.tokenInput.Focus()
			return m, textinput.Blink
		case "esc":
			m.phase = phaseWelcome
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.aliasInput, cmd = m.aliasInput.Update(msg)
	return m, cmd
}

func (m Model) updateToken(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			val := strings.TrimSpace(m.tokenInput.Value())
			if val == "" {
				return m, nil
			}
			m.token = val
			m.phase = phaseValidating
			return m, tea.Batch(m.spinner.Tick, m.validateToken())
		case "esc":
			m.phase = phaseAlias
			m.aliasInput.Focus()
			return m, textinput.Blink
		}
	}
	var cmd tea.Cmd
	m.tokenInput, cmd = m.tokenInput.Update(msg)
	return m, cmd
}

func (m Model) updateValidating(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case validateDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.phase = phaseDone
			return m, nil
		}
		m.servers = msg.servers

		// Store token and servers.
		if err := m.store.SetCredential("hetzner", m.alias, "api_token", m.token); err != nil {
			m.err = err
			m.phase = phaseDone
			return m, nil
		}

		cfg, _ := m.store.LoadConfig()
		for _, s := range m.servers {
			cfg.Servers = append(cfg.Servers, config.Server{
				Name:           s.Name,
				IP:             s.PublicNet.IPv4.IP,
				HetznerProject: m.alias,
				HetznerID:      s.ID,
			})
		}
		if err := m.store.SaveConfig(cfg); err != nil {
			m.err = err
		}

		m.phase = phaseDone
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter", "q", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) validateToken() tea.Cmd {
	token := m.token
	return func() tea.Msg {
		client := fhetzner.NewClient(token)
		if err := client.Ping(); err != nil {
			return validateDoneMsg{err: fmt.Errorf("invalid token: %w", err)}
		}
		servers, err := client.ListServers()
		if err != nil {
			return validateDoneMsg{err: fmt.Errorf("listing servers: %w", err)}
		}
		return validateDoneMsg{servers: servers}
	}
}

// View renders the current phase.
func (m Model) View() string {
	var b strings.Builder

	b.WriteString(tui.TitleStyle.Render("arnor setup"))
	b.WriteString("\n")

	switch m.phase {
	case phaseWelcome:
		b.WriteString("Welcome to arnor! No configuration found.\n\n")
		b.WriteString("This wizard will set up your first Hetzner project.\n")
		b.WriteString("You'll need a Hetzner Cloud API token.\n")
		b.WriteString(tui.HelpStyle.Render("\nenter: start  q: quit"))

	case phaseAlias:
		b.WriteString("Project alias (e.g. prod, dev, default):\n\n")
		b.WriteString(m.aliasInput.View())
		b.WriteString(tui.HelpStyle.Render("\nenter: next  esc: back"))

	case phaseToken:
		b.WriteString(renderField("Alias", m.alias))
		b.WriteString("\nHetzner API token:\n\n")
		b.WriteString(m.tokenInput.View())
		b.WriteString(tui.HelpStyle.Render("\nenter: validate  esc: back"))

	case phaseValidating:
		b.WriteString(renderField("Alias", m.alias))
		b.WriteString("\n")
		b.WriteString(m.spinner.View())
		b.WriteString(" Validating token and discovering servers...")

	case phaseDone:
		b.WriteString(renderField("Alias", m.alias))
		b.WriteString("\n")
		if m.err != nil {
			b.WriteString(tui.ErrorStyle.Render("Error: "))
			b.WriteString(m.err.Error())
		} else {
			b.WriteString(tui.SuccessStyle.Render("Setup complete!"))
			b.WriteString(fmt.Sprintf("\n\nDiscovered %d server(s).\n", len(m.servers)))
			for _, s := range m.servers {
				b.WriteString(fmt.Sprintf("  - %s (%s)\n", s.Name, s.PublicNet.IPv4.IP))
			}
			b.WriteString("\nRun 'arnor tui' again to access the main menu.")
			b.WriteString("\nUse 'arnor config add' to set Porkbun, Cloudflare, or DockerHub credentials.")
		}
		b.WriteString(tui.HelpStyle.Render("\nenter/q: exit"))
	}

	return b.String()
}

func renderField(label, value string) string {
	return tui.LabelStyle.Render(label+":") + " " + tui.ValueStyle.Render(value) + "\n"
}
