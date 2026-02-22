package servicedeploy

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/hetzner"
	"github.com/dukerupert/arnor/internal/project"
	"github.com/dukerupert/arnor/internal/service"
	"github.com/dukerupert/arnor/tui"
)

type phase int

const (
	phaseServiceName phase = iota
	phaseSelectServer
	phaseDomain
	phasePort
	phaseComposeFile
	phaseConfirm
	phaseRunning
	phaseDone
)

// progressMsg carries a step update from the Deploy goroutine.
type progressMsg struct {
	step    int
	total   int
	message string
	done    bool
	err     error
}

// usedPortsMsg carries the result of the async port scan.
type usedPortsMsg struct {
	ports []int
	err   error
}

// Model is the BubbleTea model for the service deploy screen.
type Model struct {
	phase phase

	servers      []hetzner.ServerWithProject
	serverCursor int
	store        config.Store

	textInput textinput.Model
	spinner   spinner.Model

	// Collected fields
	serviceName string
	serverName  string
	serverIP    string
	domain      string
	port        int
	composeFile string

	// Peon key resolved from server IP
	peonKey string

	// Port suggestion from docker ps scan
	suggestedPort int
	portScanning  bool

	// Progress channel for Deploy callback
	progressCh chan progressMsg

	// Current progress display
	currentStep    int
	currentTotal   int
	currentMessage string

	err error
}

// New creates a new service deploy model.
func New(servers []hetzner.ServerWithProject, store config.Store) Model {
	ti := textinput.New()
	ti.CharLimit = 128
	ti.Placeholder = "uptime-kuma"
	ti.Focus()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = tui.SpinnerStyle

	return Model{
		phase:     phaseServiceName,
		servers:   servers,
		store:     store,
		textInput: ti,
		spinner:   s,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseServiceName:
		return m.updateServiceName(msg)
	case phaseSelectServer:
		return m.updateSelectServer(msg)
	case phaseDomain:
		return m.updateDomain(msg)
	case phasePort:
		return m.updatePort(msg)
	case phaseComposeFile:
		return m.updateComposeFile(msg)
	case phaseConfirm:
		return m.updateConfirm(msg)
	case phaseRunning:
		return m.updateRunning(msg)
	case phaseDone:
		return m.updateDone(msg)
	}
	return m, nil
}

func (m Model) updateServiceName(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			val := strings.TrimSpace(m.textInput.Value())
			if val == "" {
				return m, nil
			}
			m.serviceName = val
			m.phase = phaseSelectServer
			m.textInput.Blur()
			return m, nil
		case "esc":
			return m, func() tea.Msg {
				return tui.SwitchScreenMsg{Screen: tui.ScreenMenu}
			}
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
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
			m.serverName = srv.Name
			m.serverIP = srv.PublicNet.IPv4.IP

			// Resolve peon key from store
			peonKey, err := m.store.GetPeonKey(m.serverIP)
			if err != nil {
				m.err = fmt.Errorf("peon key for %s: %w — run Server Init first", m.serverIP, err)
				m.phase = phaseDone
				return m, nil
			}
			m.peonKey = peonKey

			m.phase = phaseDomain
			m.textInput.SetValue("")
			m.textInput.Placeholder = "status.mydomain.com"
			m.textInput.Focus()
			return m, textinput.Blink
		case "esc":
			return m.goBack()
		}
	}
	return m, nil
}

func (m Model) updateDomain(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			val := strings.TrimSpace(m.textInput.Value())
			if val == "" {
				return m, nil
			}
			m.domain = val
			m.phase = phasePort
			m.textInput.SetValue("")
			m.textInput.Placeholder = "scanning ports..."
			m.portScanning = true
			m.textInput.Focus()
			return m, tea.Batch(textinput.Blink, m.scanPorts())
		case "esc":
			return m.goBack()
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) updatePort(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case usedPortsMsg:
		m.portScanning = false
		if msg.err == nil {
			m.suggestedPort = project.SuggestPort(msg.ports)
			m.textInput.SetValue(strconv.Itoa(m.suggestedPort))
			m.textInput.Placeholder = fmt.Sprintf("suggested: %d", m.suggestedPort)
		} else {
			m.textInput.Placeholder = "3000"
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			val := strings.TrimSpace(m.textInput.Value())
			if val == "" {
				return m, nil
			}
			port, err := strconv.Atoi(val)
			if err != nil {
				return m, nil
			}
			m.port = port
			m.phase = phaseComposeFile
			m.textInput.SetValue("")
			m.textInput.Placeholder = "./docker-compose.yml"
			m.textInput.Focus()
			return m, textinput.Blink
		case "esc":
			return m.goBack()
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) updateComposeFile(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			val := strings.TrimSpace(m.textInput.Value())
			if val == "" {
				val = "./docker-compose.yml"
			}
			m.composeFile = val
			m.phase = phaseConfirm
			m.textInput.Blur()
			return m, nil
		case "esc":
			return m.goBack()
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter", "y":
			m.phase = phaseRunning
			m.progressCh = make(chan progressMsg, 1)
			return m, tea.Batch(
				m.spinner.Tick,
				m.runDeploy(),
				m.waitForProgress(),
			)
		case "esc", "n":
			return m.goBack()
		}
	}
	return m, nil
}

func (m Model) updateRunning(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progressMsg:
		if msg.done {
			m.err = msg.err
			m.phase = phaseDone
			return m, nil
		}
		m.currentStep = msg.step
		m.currentTotal = msg.total
		m.currentMessage = msg.message
		return m, m.waitForProgress()
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
		case "enter":
			return m, func() tea.Msg {
				return tui.SwitchScreenMsg{Screen: tui.ScreenMenu}
			}
		case "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

// goBack navigates to the previous phase.
func (m Model) goBack() (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseServiceName:
		return m, func() tea.Msg {
			return tui.SwitchScreenMsg{Screen: tui.ScreenMenu}
		}
	case phaseSelectServer:
		m.phase = phaseServiceName
		m.textInput.SetValue(m.serviceName)
		m.textInput.Placeholder = "uptime-kuma"
		m.textInput.Focus()
		return m, textinput.Blink
	case phaseDomain:
		m.phase = phaseSelectServer
		m.textInput.Blur()
		return m, nil
	case phasePort:
		m.phase = phaseDomain
		m.textInput.SetValue(m.domain)
		m.textInput.Placeholder = "status.mydomain.com"
		m.textInput.Focus()
		return m, textinput.Blink
	case phaseComposeFile:
		m.phase = phasePort
		m.textInput.SetValue(strconv.Itoa(m.port))
		m.textInput.Placeholder = "3000"
		m.textInput.Focus()
		return m, textinput.Blink
	case phaseConfirm:
		m.phase = phaseComposeFile
		m.textInput.SetValue(m.composeFile)
		m.textInput.Placeholder = "./docker-compose.yml"
		m.textInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

// runDeploy launches service.Deploy in a goroutine and pushes progress to the channel.
func (m Model) runDeploy() tea.Cmd {
	ch := m.progressCh
	params := service.DeployParams{
		ServiceName: m.serviceName,
		ServerName:  m.serverName,
		Domain:      m.domain,
		Port:        m.port,
		ComposeFile: m.composeFile,
		PeonKey:     m.peonKey,
		Store:       m.store,
		OnProgress: func(step, total int, message string) {
			ch <- progressMsg{step: step, total: total, message: message}
		},
	}

	return func() tea.Msg {
		err := service.Deploy(params)
		ch <- progressMsg{done: true, err: err}
		return nil
	}
}

// waitForProgress blocks on the progress channel and returns the next message.
func (m Model) waitForProgress() tea.Cmd {
	ch := m.progressCh
	return func() tea.Msg {
		return <-ch
	}
}

// scanPorts returns a tea.Cmd that SSHs to the server and scans used Docker ports.
func (m Model) scanPorts() tea.Cmd {
	serverIP := m.serverIP
	peonKey := m.peonKey
	return func() tea.Msg {
		ports, err := project.GetUsedPorts(serverIP, peonKey)
		return usedPortsMsg{ports: ports, err: err}
	}
}

// View renders the current phase.
func (m Model) View() string {
	var b strings.Builder

	b.WriteString(tui.TitleStyle.Render("Service Deploy"))
	b.WriteString("\n")

	// Running summary of entered fields
	summary := m.renderSummary()
	if summary != "" {
		b.WriteString(summary)
	}

	switch m.phase {
	case phaseServiceName:
		b.WriteString("Service name:\n\n")
		b.WriteString(m.textInput.View())
		b.WriteString(tui.HelpStyle.Render("\nenter: next  esc: menu"))

	case phaseSelectServer:
		b.WriteString("\nSelect a server:\n\n")
		for i, srv := range m.servers {
			line := fmt.Sprintf("  %s  %s  (%s)", srv.Name, srv.PublicNet.IPv4.IP, srv.ProjectAlias)
			if i == m.serverCursor {
				line = tui.CursorStyle.Render("> " + fmt.Sprintf("%s  %s  (%s)", srv.Name, srv.PublicNet.IPv4.IP, srv.ProjectAlias))
			}
			b.WriteString(line + "\n")
		}
		b.WriteString(tui.HelpStyle.Render("\nj/k: navigate  enter: select  esc: back"))

	case phaseDomain:
		b.WriteString("\nDomain:\n\n")
		b.WriteString(m.textInput.View())
		b.WriteString(tui.HelpStyle.Render("\nenter: next  esc: back"))

	case phasePort:
		b.WriteString("\nPort:\n\n")
		b.WriteString(m.textInput.View())
		if m.portScanning {
			b.WriteString(tui.HelpStyle.Render("\nscanning docker ports..."))
		}
		b.WriteString(tui.HelpStyle.Render("\nenter: next  esc: back"))

	case phaseComposeFile:
		b.WriteString("\nCompose file path:\n\n")
		b.WriteString(m.textInput.View())
		b.WriteString(tui.HelpStyle.Render("\nempty = ./docker-compose.yml"))
		b.WriteString(tui.HelpStyle.Render("\nenter: next  esc: back"))

	case phaseConfirm:
		b.WriteString("\n")
		b.WriteString(tui.CursorStyle.Render("Deploy this service?"))
		b.WriteString(tui.HelpStyle.Render("\nenter/y: run  esc/n: back"))

	case phaseRunning:
		b.WriteString("\n")
		b.WriteString(m.spinner.View())
		if m.currentStep > 0 {
			b.WriteString(fmt.Sprintf(" Step %d/%d: %s", m.currentStep, m.currentTotal, m.currentMessage))
		} else {
			b.WriteString(" Starting...")
		}

	case phaseDone:
		b.WriteString("\n")
		if m.err != nil {
			b.WriteString(tui.ErrorStyle.Render("Error: "))
			b.WriteString(m.err.Error())
		} else {
			b.WriteString(tui.SuccessStyle.Render("Success!"))
			b.WriteString(fmt.Sprintf("\n\nService %s deployed to %s.", m.serviceName, m.domain))
		}
		b.WriteString(tui.HelpStyle.Render("\nenter: menu  q: quit"))
	}

	return b.String()
}

// renderSummary shows previously entered fields as a running summary.
func (m Model) renderSummary() string {
	var b strings.Builder
	render := func(label, value string) {
		b.WriteString(tui.LabelStyle.Render(label+":") + " " + tui.ValueStyle.Render(value) + "\n")
	}

	if m.serviceName != "" && m.phase > phaseServiceName {
		render("Service", m.serviceName)
	}
	if m.serverName != "" && m.phase > phaseSelectServer {
		render("Server", fmt.Sprintf("%s (%s)", m.serverName, m.serverIP))
	}
	if m.domain != "" && m.phase > phaseDomain {
		render("Domain", m.domain)
	}
	if m.port > 0 && m.phase > phasePort {
		render("Port", strconv.Itoa(m.port))
	}
	if m.composeFile != "" && m.phase > phaseComposeFile {
		render("Compose", m.composeFile)
	}

	return b.String()
}
