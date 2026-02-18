package serverinit

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dukerupert/arnor/internal/peon"
	"github.com/dukerupert/arnor/tui"
)

type phase int

const (
	phaseHost phase = iota
	phaseUser
	phaseSudoPassword
	phaseConfirm
	phaseRunning
	phasePassphrase
	phaseDone
)

// Messages for the tea event loop.
type runDoneMsg struct {
	key string
	err error
}

type saveDoneMsg struct {
	result *peon.SaveResult
	err    error
}

type passphraseRequestMsg struct{}

// Model is the BubbleTea model for the server init screen.
type Model struct {
	phase phase

	hostInput       textinput.Model
	userInput       textinput.Model
	sudoInput       textinput.Model
	passphraseInput textinput.Model
	spinner         spinner.Model

	host         string
	user         string
	sudoPassword string

	// Channel pair for passphrase callback from the SSH goroutine.
	passphraseWait chan struct{}       // SSH goroutine signals it needs a passphrase
	passphraseCh   chan passphraseResp // TUI sends the passphrase back

	key    string
	result *peon.SaveResult
	err    error
}

type passphraseResp struct {
	passphrase []byte
	err        error
}

// New creates a new server init model.
func New() Model {
	hi := textinput.New()
	hi.Placeholder = "e.g. 5.78.122.182"
	hi.Focus()
	hi.CharLimit = 253

	ui := textinput.New()
	ui.Placeholder = "root"
	ui.SetValue("root")
	ui.CharLimit = 32

	si := textinput.New()
	si.Placeholder = "sudo password"
	si.EchoMode = textinput.EchoPassword
	si.CharLimit = 128

	pi := textinput.New()
	pi.Placeholder = "SSH key passphrase"
	pi.EchoMode = textinput.EchoPassword
	pi.CharLimit = 128

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = tui.SpinnerStyle

	return Model{
		phase:          phaseHost,
		hostInput:      hi,
		userInput:      ui,
		sudoInput:      si,
		passphraseInput: pi,
		spinner:        s,
		passphraseWait: make(chan struct{}, 1),
		passphraseCh:   make(chan passphraseResp, 1),
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseHost:
		return m.updateHost(msg)
	case phaseUser:
		return m.updateUser(msg)
	case phaseSudoPassword:
		return m.updateSudo(msg)
	case phaseConfirm:
		return m.updateConfirm(msg)
	case phaseRunning:
		return m.updateRunning(msg)
	case phasePassphrase:
		return m.updatePassphrase(msg)
	case phaseDone:
		return m.updateDone(msg)
	}
	return m, nil
}

func (m Model) updateHost(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			val := strings.TrimSpace(m.hostInput.Value())
			if val == "" {
				return m, nil
			}
			m.host = val
			m.phase = phaseUser
			m.userInput.Focus()
			return m, textinput.Blink
		case "esc":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.hostInput, cmd = m.hostInput.Update(msg)
	return m, cmd
}

func (m Model) updateUser(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			val := strings.TrimSpace(m.userInput.Value())
			if val == "" {
				val = "root"
			}
			m.user = val
			if m.user != "root" {
				m.phase = phaseSudoPassword
				m.sudoInput.Focus()
				return m, textinput.Blink
			}
			m.phase = phaseConfirm
			return m, nil
		case "esc":
			m.phase = phaseHost
			m.hostInput.Focus()
			return m, textinput.Blink
		}
	}
	var cmd tea.Cmd
	m.userInput, cmd = m.userInput.Update(msg)
	return m, cmd
}

func (m Model) updateSudo(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			m.sudoPassword = m.sudoInput.Value()
			m.phase = phaseConfirm
			return m, nil
		case "esc":
			m.phase = phaseUser
			m.userInput.Focus()
			return m, textinput.Blink
		}
	}
	var cmd tea.Cmd
	m.sudoInput, cmd = m.sudoInput.Update(msg)
	return m, cmd
}

func (m Model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter", "y":
			m.phase = phaseRunning
			return m, tea.Batch(
				m.spinner.Tick,
				m.runRemote(),
				m.waitForPassphraseRequest(),
			)
		case "esc", "n":
			// Go back to user input
			if m.user != "root" {
				m.phase = phaseSudoPassword
				m.sudoInput.Focus()
				return m, textinput.Blink
			}
			m.phase = phaseUser
			m.userInput.Focus()
			return m, textinput.Blink
		}
	}
	return m, nil
}

func (m Model) updateRunning(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case runDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.phase = phaseDone
			return m, nil
		}
		m.key = msg.key
		return m, m.saveKey()
	case saveDoneMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.result = msg.result
		}
		m.phase = phaseDone
		return m, nil
	case passphraseRequestMsg:
		m.phase = phasePassphrase
		m.passphraseInput.SetValue("")
		m.passphraseInput.Focus()
		return m, textinput.Blink
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updatePassphrase(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			val := m.passphraseInput.Value()
			m.passphraseCh <- passphraseResp{passphrase: []byte(val)}
			m.phase = phaseRunning
			return m, tea.Batch(
				m.spinner.Tick,
				m.waitForPassphraseRequest(),
			)
		case "esc":
			m.passphraseCh <- passphraseResp{err: fmt.Errorf("passphrase entry cancelled")}
			m.phase = phaseRunning
			return m, m.spinner.Tick
		}
	}
	var cmd tea.Cmd
	m.passphraseInput, cmd = m.passphraseInput.Update(msg)
	return m, cmd
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

// runRemote launches peon.RunRemote in a goroutine and returns the result as a message.
func (m Model) runRemote() tea.Cmd {
	passphraseWait := m.passphraseWait
	passphraseCh := m.passphraseCh
	host := m.host
	user := m.user
	sudoPassword := m.sudoPassword

	return func() tea.Msg {
		auth := peon.SSHAuth{
			SudoPassword: sudoPassword,
			KeyPassphraseFunc: func() ([]byte, error) {
				passphraseWait <- struct{}{}
				resp := <-passphraseCh
				return resp.passphrase, resp.err
			},
		}
		key, err := peon.RunRemote(host, user, auth)
		return runDoneMsg{key: key, err: err}
	}
}

// waitForPassphraseRequest blocks until the SSH goroutine needs a passphrase.
func (m Model) waitForPassphraseRequest() tea.Cmd {
	ch := m.passphraseWait
	return func() tea.Msg {
		<-ch
		return passphraseRequestMsg{}
	}
}

// saveKey saves the peon key to disk.
func (m Model) saveKey() tea.Cmd {
	host := m.host
	key := m.key
	return func() tea.Msg {
		result, err := peon.SavePeonKey(host, key)
		return saveDoneMsg{result: result, err: err}
	}
}

// View renders the current phase.
func (m Model) View() string {
	var b strings.Builder

	b.WriteString(tui.TitleStyle.Render("Server Init"))
	b.WriteString("\n")

	switch m.phase {
	case phaseHost:
		b.WriteString("Enter the server hostname or IP:\n\n")
		b.WriteString(m.hostInput.View())
		b.WriteString(tui.HelpStyle.Render("\nenter: next  esc: quit"))

	case phaseUser:
		b.WriteString(renderField("Host", m.host))
		b.WriteString("\nSSH user:\n\n")
		b.WriteString(m.userInput.View())
		b.WriteString(tui.HelpStyle.Render("\nenter: next  esc: back"))

	case phaseSudoPassword:
		b.WriteString(renderField("Host", m.host))
		b.WriteString(renderField("User", m.user))
		b.WriteString(fmt.Sprintf("\nSudo password for %s:\n\n", m.user))
		b.WriteString(m.sudoInput.View())
		b.WriteString(tui.HelpStyle.Render("\nenter: next  esc: back"))

	case phaseConfirm:
		b.WriteString(renderField("Host", m.host))
		b.WriteString(renderField("User", m.user))
		if m.user != "root" {
			b.WriteString(renderField("Sudo", "********"))
		}
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("Bootstrap peon on this server?"))
		b.WriteString(tui.HelpStyle.Render("\nenter/y: run  esc/n: back"))

	case phaseRunning:
		b.WriteString(renderField("Host", m.host))
		b.WriteString(renderField("User", m.user))
		b.WriteString("\n")
		b.WriteString(m.spinner.View())
		b.WriteString(" Bootstrapping peon...")

	case phasePassphrase:
		b.WriteString(renderField("Host", m.host))
		b.WriteString(renderField("User", m.user))
		b.WriteString("\nSSH key passphrase required:\n\n")
		b.WriteString(m.passphraseInput.View())
		b.WriteString(tui.HelpStyle.Render("\nenter: submit  esc: cancel"))

	case phaseDone:
		b.WriteString(renderField("Host", m.host))
		b.WriteString(renderField("User", m.user))
		b.WriteString("\n")
		if m.err != nil {
			b.WriteString(tui.ErrorStyle.Render("Error: "))
			b.WriteString(m.err.Error())
		} else {
			b.WriteString(tui.SuccessStyle.Render("Success!"))
			b.WriteString("\n\n")
			b.WriteString(renderField("Key saved", m.result.KeyPath))
			b.WriteString(renderField("Env var", m.result.EnvKey))
			b.WriteString(renderField("Env file", m.result.EnvPath))
		}
		b.WriteString(tui.HelpStyle.Render("\nenter/q: quit"))
	}

	return b.String()
}

func renderField(label, value string) string {
	return tui.LabelStyle.Render(label+":") + " " + tui.ValueStyle.Render(value) + "\n"
}
