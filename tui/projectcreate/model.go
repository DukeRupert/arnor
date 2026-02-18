package projectcreate

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dukerupert/arnor/internal/hetzner"
	"github.com/dukerupert/arnor/internal/project"
	"github.com/dukerupert/arnor/tui"
)

type phase int

const (
	phaseSelectRepo phase = iota
	phaseSelectServer
	phaseSelectEnv
	phaseDomain
	phasePort
	phaseConfirm
	phaseRunning
	phaseDone
)

var envChoices = []string{"dev", "prod"}

// progressMsg carries a step update from the Setup goroutine.
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

// Model is the BubbleTea model for the project create screen.
type Model struct {
	phase phase

	repos      []project.GitHubRepo
	repoCursor int

	servers      []hetzner.ServerWithProject
	serverCursor int
	envCursor    int

	textInput textinput.Model
	spinner   spinner.Model

	// Collected fields
	projectName string
	repo        string // owner/repo format
	serverName  string
	serverIP    string
	envName     string
	domain      string
	port        int

	// Peon key resolved from server IP
	peonKey string

	// Port suggestion from docker ps scan
	suggestedPort int
	portScanning  bool

	// Progress channel for Setup callback
	progressCh chan progressMsg

	// Current progress display
	currentStep    int
	currentTotal   int
	currentMessage string

	err error
}

// New creates a new project create model.
func New(repos []project.GitHubRepo, servers []hetzner.ServerWithProject) Model {
	ti := textinput.New()
	ti.CharLimit = 128

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = tui.SpinnerStyle

	return Model{
		phase:   phaseSelectRepo,
		repos:   repos,
		servers: servers,
		textInput: ti,
		spinner:   s,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseSelectRepo:
		return m.updateSelectRepo(msg)
	case phaseSelectServer:
		return m.updateSelectServer(msg)
	case phaseSelectEnv:
		return m.updateSelectEnv(msg)
	case phaseDomain:
		return m.updateDomain(msg)
	case phasePort:
		return m.updatePort(msg)
	case phaseConfirm:
		return m.updateConfirm(msg)
	case phaseRunning:
		return m.updateRunning(msg)
	case phaseDone:
		return m.updateDone(msg)
	}
	return m, nil
}

// updateTextInput handles generic text input phases with a next callback.
func (m Model) updateTextInput(msg tea.Msg, next func(string) (tea.Model, tea.Cmd)) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			val := strings.TrimSpace(m.textInput.Value())
			if val == "" {
				return m, nil
			}
			return next(val)
		case "esc":
			return m.goBack()
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) updateSelectRepo(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.repoCursor > 0 {
				m.repoCursor--
			}
		case "down", "j":
			if m.repoCursor < len(m.repos)-1 {
				m.repoCursor++
			}
		case "enter":
			if len(m.repos) == 0 {
				return m, nil
			}
			selected := m.repos[m.repoCursor]
			m.repo = selected.NameWithOwner
			m.projectName = selected.Name
			m.phase = phaseSelectServer
			return m, nil
		case "esc":
			return m, func() tea.Msg {
				return tui.SwitchScreenMsg{Screen: tui.ScreenMenu}
			}
		}
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
			m.serverName = srv.Name
			m.serverIP = srv.PublicNet.IPv4.IP

			// Resolve peon key from server IP
			peonKey, err := resolvePeonKey(m.serverIP)
			if err != nil {
				m.err = fmt.Errorf("peon key for %s: %w — run 'arnor tui' > Server Init first", m.serverIP, err)
				m.phase = phaseDone
				return m, nil
			}
			m.peonKey = peonKey

			m.phase = phaseSelectEnv
			return m, nil
		case "esc":
			return m.goBack()
		}
	}
	return m, nil
}

func (m Model) updateSelectEnv(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.envCursor > 0 {
				m.envCursor--
			}
		case "down", "j":
			if m.envCursor < len(envChoices)-1 {
				m.envCursor++
			}
		case "enter":
			m.envName = envChoices[m.envCursor]
			m.phase = phaseDomain
			m.textInput.SetValue("")
			if m.envName == "dev" {
				m.textInput.Placeholder = m.projectName + ".angmar.dev"
			} else {
				m.textInput.Placeholder = "example.com"
			}
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
			if val == "" && m.envName == "dev" {
				val = m.projectName + ".angmar.dev"
			}
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
				m.runSetup(),
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
	case phaseSelectRepo:
		return m, func() tea.Msg {
			return tui.SwitchScreenMsg{Screen: tui.ScreenMenu}
		}
	case phaseSelectServer:
		m.phase = phaseSelectRepo
		return m, nil
	case phaseSelectEnv:
		m.phase = phaseSelectServer
		return m, nil
	case phaseDomain:
		m.phase = phaseSelectEnv
		m.textInput.Blur()
		return m, nil
	case phasePort:
		m.phase = phaseDomain
		m.textInput.SetValue(m.domain)
		if m.envName == "dev" {
			m.textInput.Placeholder = m.projectName + ".angmar.dev"
		} else {
			m.textInput.Placeholder = "example.com"
		}
		m.textInput.Focus()
		return m, textinput.Blink
	case phaseConfirm:
		m.phase = phasePort
		m.textInput.SetValue(strconv.Itoa(m.port))
		m.textInput.Placeholder = "3000"
		m.textInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

// runSetup launches project.Setup in a goroutine and pushes progress to the channel.
func (m Model) runSetup() tea.Cmd {
	ch := m.progressCh
	params := project.SetupParams{
		ProjectName: m.projectName,
		Repo:        m.repo,
		ServerName:  m.serverName,
		EnvName:     m.envName,
		Domain:      m.domain,
		Port:        m.port,
		PeonKey:     m.peonKey,
		OnProgress: func(step, total int, message string) {
			ch <- progressMsg{step: step, total: total, message: message}
		},
	}

	return func() tea.Msg {
		err := project.Setup(params)
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

// resolvePeonKey reads the peon SSH key for a server IP from the env var / file path convention.
func resolvePeonKey(serverIP string) (string, error) {
	envKey := "PEON_SSH_KEY_" + strings.ReplaceAll(serverIP, ".", "_")
	keyPath := os.Getenv(envKey)
	if keyPath == "" {
		return "", fmt.Errorf("env var %s is not set", envKey)
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", keyPath, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// View renders the current phase.
func (m Model) View() string {
	var b strings.Builder

	b.WriteString(tui.TitleStyle.Render("Project Create"))
	b.WriteString("\n")

	// Running summary of entered fields
	summary := m.renderSummary()
	if summary != "" {
		b.WriteString(summary)
	}

	switch m.phase {
	case phaseSelectRepo:
		b.WriteString("Select a repository:\n\n")
		for i, repo := range m.repos {
			line := fmt.Sprintf("  %s", repo.NameWithOwner)
			if i == m.repoCursor {
				line = tui.CursorStyle.Render("> " + repo.NameWithOwner)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString(tui.HelpStyle.Render("\nj/k: navigate  enter: select  esc: menu"))

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

	case phaseSelectEnv:
		b.WriteString("\nEnvironment:\n\n")
		for i, env := range envChoices {
			line := fmt.Sprintf("  %s", env)
			if i == m.envCursor {
				line = tui.CursorStyle.Render("> " + env)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString(tui.HelpStyle.Render("\nj/k: navigate  enter: select  esc: back"))

	case phaseDomain:
		b.WriteString("\nDomain:\n\n")
		b.WriteString(m.textInput.View())
		if m.envName == "dev" {
			b.WriteString(tui.HelpStyle.Render(fmt.Sprintf("\nempty = %s.angmar.dev", m.projectName)))
		}
		b.WriteString(tui.HelpStyle.Render("\nenter: next  esc: back"))

	case phasePort:
		b.WriteString("\nPort:\n\n")
		b.WriteString(m.textInput.View())
		if m.portScanning {
			b.WriteString(tui.HelpStyle.Render("\nscanning docker ports..."))
		}
		b.WriteString(tui.HelpStyle.Render("\nenter: next  esc: back"))

	case phaseConfirm:
		b.WriteString("\n")
		b.WriteString(tui.CursorStyle.Render("Create this project?"))
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
			b.WriteString(fmt.Sprintf("\n\n%s %s environment is ready.", m.projectName, m.envName))
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

	if m.repo != "" && m.phase > phaseSelectRepo {
		render("Repo", m.repo)
	}
	if m.serverName != "" && m.phase > phaseSelectServer {
		render("Server", fmt.Sprintf("%s (%s)", m.serverName, m.serverIP))
	}
	if m.envName != "" && m.phase > phaseSelectEnv {
		render("Env", m.envName)
	}
	if m.domain != "" && m.phase > phaseDomain {
		render("Domain", m.domain)
	}
	if m.port > 0 && m.phase > phasePort {
		render("Port", strconv.Itoa(m.port))
	}

	return b.String()
}
