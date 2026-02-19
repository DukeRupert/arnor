package credentials

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/tui"
	"github.com/dukerupert/shadowfax/pkg/porkbun"
)

// fieldSpec describes one credential field for a service.
type fieldSpec struct {
	key         string
	label       string
	placeholder string
	masked      bool
}

// serviceSpec describes how to configure one service.
type serviceSpec struct {
	name        string
	service     string // store key
	fields      []fieldSpec
	canValidate bool
}

var services = []serviceSpec{
	{
		name:    "Porkbun",
		service: "porkbun",
		fields: []fieldSpec{
			{key: "api_key", label: "API Key", placeholder: "pk1_...", masked: true},
			{key: "secret_key", label: "Secret Key", placeholder: "sk1_...", masked: true},
		},
		canValidate: true,
	},
	{
		name:    "Cloudflare",
		service: "cloudflare",
		fields: []fieldSpec{
			{key: "account_id", label: "Account ID", placeholder: "8c21e667...", masked: false},
			{key: "api_token", label: "API Token", placeholder: "token", masked: true},
		},
		canValidate: true,
	},
	{
		name:    "DockerHub",
		service: "dockerhub",
		fields: []fieldSpec{
			{key: "username", label: "Username", placeholder: "username", masked: false},
			{key: "password", label: "Password", placeholder: "password", masked: true},
		},
		canValidate: false,
	},
}

type phase int

const (
	phaseSelectService phase = iota
	phaseField
	phaseValidating
	phaseDone
)

type validateDoneMsg struct {
	err error
}

// Model is the BubbleTea model for the credentials configuration screen.
type Model struct {
	phase   phase
	store   config.Store
	cursor  int
	statuses []bool // true = configured, per service

	selectedIdx int
	fieldIdx    int
	fieldValues []string
	textInput   textinput.Model
	spinner     spinner.Model
	err         error
}

// New creates a new credentials model. Loads status synchronously.
func New(store config.Store) Model {
	statuses := make([]bool, len(services))
	for i, svc := range services {
		creds, _ := store.ListCredentials(svc.service)
		statuses[i] = len(creds) > 0
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = tui.SpinnerStyle

	return Model{
		phase:    phaseSelectService,
		store:    store,
		statuses: statuses,
		spinner:  s,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseSelectService:
		return m.updateSelectService(msg)
	case phaseField:
		return m.updateField(msg)
	case phaseValidating:
		return m.updateValidating(msg)
	case phaseDone:
		return m.updateDone(msg)
	}
	return m, nil
}

func (m Model) updateSelectService(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(services)-1 {
			m.cursor++
		}
	case "enter":
		m.selectedIdx = m.cursor
		m.fieldIdx = 0
		m.fieldValues = make([]string, len(services[m.selectedIdx].fields))
		m.err = nil
		m.phase = phaseField
		m.textInput = initFieldInput(m.selectedIdx, m.fieldIdx)
		return m, textinput.Blink
	case "esc", "q":
		return m, func() tea.Msg {
			return tui.SwitchScreenMsg{Screen: tui.ScreenMenu}
		}
	}
	return m, nil
}

func (m Model) updateField(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			val := strings.TrimSpace(m.textInput.Value())
			if val == "" {
				return m, nil
			}
			m.fieldValues[m.fieldIdx] = val

			svc := services[m.selectedIdx]
			if m.fieldIdx < len(svc.fields)-1 {
				m.fieldIdx++
				m.textInput = initFieldInput(m.selectedIdx, m.fieldIdx)
				return m, textinput.Blink
			}

			// All fields collected — save then validate or finish.
			for i, f := range svc.fields {
				if err := m.store.SetCredential(svc.service, "default", f.key, m.fieldValues[i]); err != nil {
					m.err = err
					m.phase = phaseDone
					return m, nil
				}
			}

			if svc.canValidate {
				m.phase = phaseValidating
				return m, tea.Batch(m.spinner.Tick, m.validate())
			}

			m.statuses[m.selectedIdx] = true
			m.phase = phaseDone
			return m, nil

		case "esc":
			if m.fieldIdx > 0 {
				m.fieldIdx--
				m.textInput = initFieldInput(m.selectedIdx, m.fieldIdx)
				return m, textinput.Blink
			}
			m.phase = phaseSelectService
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) updateValidating(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case validateDoneMsg:
		if msg.err != nil {
			// Remove saved credentials on validation failure.
			svc := services[m.selectedIdx]
			_ = m.store.DeleteCredential(svc.service, "default")
			m.err = msg.err
		} else {
			m.statuses[m.selectedIdx] = true
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
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "enter":
		m.phase = phaseSelectService
		m.err = nil
		return m, nil
	case "esc", "q":
		return m, func() tea.Msg {
			return tui.SwitchScreenMsg{Screen: tui.ScreenMenu}
		}
	}
	return m, nil
}

// initFieldInput creates a focused text input for the current field.
func initFieldInput(svcIdx, fieldIdx int) textinput.Model {
	f := services[svcIdx].fields[fieldIdx]
	ti := textinput.New()
	ti.Placeholder = f.placeholder
	ti.CharLimit = 256
	if f.masked {
		ti.EchoMode = textinput.EchoPassword
	}
	ti.Focus()
	return ti
}

// validate runs the appropriate validation for the selected service.
func (m Model) validate() tea.Cmd {
	svc := services[m.selectedIdx]
	values := make([]string, len(m.fieldValues))
	copy(values, m.fieldValues)

	return func() tea.Msg {
		switch svc.service {
		case "porkbun":
			client := porkbun.NewClient(values[0], values[1])
			if _, err := client.Ping(); err != nil {
				return validateDoneMsg{err: fmt.Errorf("porkbun validation failed: %w", err)}
			}
		case "cloudflare":
			// Use account-scoped verify endpoint (account_id=values[0], api_token=values[1]).
			if err := verifyCFToken(values[0], values[1]); err != nil {
				return validateDoneMsg{err: fmt.Errorf("cloudflare validation failed: %w", err)}
			}
		}
		return validateDoneMsg{}
	}
}

// verifyCFToken validates a Cloudflare API token using the account-scoped endpoint.
func verifyCFToken(accountID, token string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/tokens/verify", accountID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	if !result.Success {
		if len(result.Errors) > 0 {
			return fmt.Errorf("%s (code %d)", result.Errors[0].Message, result.Errors[0].Code)
		}
		return fmt.Errorf("verification failed")
	}
	if result.Result.Status != "active" {
		return fmt.Errorf("token status: %s", result.Result.Status)
	}
	return nil
}

// View renders the current phase.
func (m Model) View() string {
	var b strings.Builder

	b.WriteString(tui.TitleStyle.Render("credentials"))
	b.WriteString("\n")

	switch m.phase {
	case phaseSelectService:
		b.WriteString("Select a service to configure:\n\n")
		for i, svc := range services {
			status := tui.ErrorStyle.Render("not configured")
			if m.statuses[i] {
				status = tui.SuccessStyle.Render("configured")
			}
			line := fmt.Sprintf("    %s  %s", svc.name, status)
			if i == m.cursor {
				line = tui.CursorStyle.Render(fmt.Sprintf("  > %s", svc.name)) + "  " + status
			}
			b.WriteString(line + "\n")
		}
		b.WriteString(tui.HelpStyle.Render("\nj/k: navigate  enter: select  esc: menu"))

	case phaseField:
		svc := services[m.selectedIdx]
		b.WriteString(fmt.Sprintf("Configure %s\n\n", svc.name))

		// Show previously entered fields as summary.
		for i := 0; i < m.fieldIdx; i++ {
			display := "********"
			if !svc.fields[i].masked {
				display = m.fieldValues[i]
			}
			b.WriteString(renderField(svc.fields[i].label, display))
		}

		f := svc.fields[m.fieldIdx]
		b.WriteString(fmt.Sprintf("%s:\n\n", f.label))
		b.WriteString(m.textInput.View())
		b.WriteString(tui.HelpStyle.Render("\nenter: next  esc: back"))

	case phaseValidating:
		svc := services[m.selectedIdx]
		b.WriteString(fmt.Sprintf("Configure %s\n\n", svc.name))
		b.WriteString(m.spinner.View())
		b.WriteString(fmt.Sprintf(" Validating %s credentials...", svc.name))

	case phaseDone:
		svc := services[m.selectedIdx]
		b.WriteString(fmt.Sprintf("Configure %s\n\n", svc.name))
		if m.err != nil {
			b.WriteString(tui.ErrorStyle.Render("Error: "))
			b.WriteString(m.err.Error())
		} else {
			b.WriteString(tui.SuccessStyle.Render(fmt.Sprintf("%s credentials saved!", svc.name)))
		}
		b.WriteString(tui.HelpStyle.Render("\nenter: back to services  esc: menu"))
	}

	return b.String()
}

func renderField(label, value string) string {
	return tui.LabelStyle.Render(label+":") + " " + tui.ValueStyle.Render(value) + "\n"
}
