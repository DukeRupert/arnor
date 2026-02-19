package projectinspect

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/project"
	"github.com/dukerupert/arnor/tui"
)

type phase int

const (
	phaseSelectProject phase = iota
	phaseLoading
	phaseDisplay
)

type inspectDoneMsg struct {
	secrets []project.GitHubSecret
	runs    []project.WorkflowRun
	err     error
}

// Model is the BubbleTea model for the project inspect screen.
type Model struct {
	phase phase

	projects []config.Project
	cursor   int

	selectedProject config.Project
	secrets         []project.GitHubSecret
	runs            []project.WorkflowRun

	spinner spinner.Model
	err     error
}

// New creates a new project inspect model.
func New(projects []config.Project) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = tui.SpinnerStyle

	return Model{
		phase:    phaseSelectProject,
		projects: projects,
		spinner:  s,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseSelectProject:
		return m.updateSelectProject(msg)
	case phaseLoading:
		return m.updateLoading(msg)
	case phaseDisplay:
		return m.updateDisplay(msg)
	}
	return m, nil
}

func (m Model) updateSelectProject(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.projects) == 0 {
				return m, nil
			}
			m.selectedProject = m.projects[m.cursor]
			m.phase = phaseLoading
			return m, tea.Batch(m.spinner.Tick, m.fetchInspectData())
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
	case inspectDoneMsg:
		m.secrets = msg.secrets
		m.runs = msg.runs
		m.err = msg.err
		m.phase = phaseDisplay
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateDisplay(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.phase = phaseSelectProject
			m.err = nil
			m.secrets = nil
			m.runs = nil
			return m, nil
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

func (m Model) fetchInspectData() tea.Cmd {
	repo := m.selectedProject.Repo
	return func() tea.Msg {
		secrets, secretsErr := project.ListGitHubSecrets(repo)
		runs, runsErr := project.ListWorkflowRuns(repo, 10)

		// Report first error encountered.
		var err error
		if secretsErr != nil {
			err = secretsErr
		} else if runsErr != nil {
			err = runsErr
		}

		return inspectDoneMsg{
			secrets: secrets,
			runs:    runs,
			err:     err,
		}
	}
}

// View renders the current phase.
func (m Model) View() string {
	var b strings.Builder

	b.WriteString(tui.TitleStyle.Render("Project Inspect"))
	b.WriteString("\n")

	switch m.phase {
	case phaseSelectProject:
		if len(m.projects) == 0 {
			b.WriteString("No projects configured.\n")
			b.WriteString(tui.HelpStyle.Render("\nesc: back"))
			break
		}
		b.WriteString("Select a project:\n\n")
		for i, p := range m.projects {
			line := fmt.Sprintf("  %s  (%s)", p.Name, p.Repo)
			if i == m.cursor {
				line = tui.CursorStyle.Render(fmt.Sprintf("> %s  (%s)", p.Name, p.Repo))
			}
			b.WriteString(line + "\n")
		}
		b.WriteString(tui.HelpStyle.Render("\nj/k: navigate  enter: select  esc: back"))

	case phaseLoading:
		b.WriteString(renderField("Project", m.selectedProject.Name))
		b.WriteString(renderField("Repo", m.selectedProject.Repo))
		b.WriteString("\n")
		b.WriteString(m.spinner.View())
		b.WriteString(" Fetching secrets and workflow runs...")

	case phaseDisplay:
		b.WriteString(renderField("Project", m.selectedProject.Name))
		b.WriteString(renderField("Repo", m.selectedProject.Repo))
		b.WriteString("\n")

		if m.err != nil {
			b.WriteString(tui.ErrorStyle.Render("Error: "))
			b.WriteString(m.err.Error())
			b.WriteString("\n\n")
		}

		// Secrets section
		b.WriteString(tui.LabelStyle.Render("── Secrets ──────────────────────"))
		b.WriteString("\n")
		if len(m.secrets) == 0 {
			b.WriteString("  (none)\n")
		} else {
			for _, s := range m.secrets {
				updated := s.UpdatedAt
				if len(updated) >= 10 {
					updated = updated[:10]
				}
				b.WriteString(fmt.Sprintf("  %-28s %s\n", s.Name, tui.HelpStyle.Render(updated)))
			}
		}

		b.WriteString("\n")

		// Runs section
		b.WriteString(tui.LabelStyle.Render("── Recent Runs ──────────────────"))
		b.WriteString("\n")
		if len(m.runs) == 0 {
			b.WriteString("  (none)\n")
		} else {
			for _, r := range m.runs {
				icon := "◎"
				if r.Status == "completed" {
					if r.Conclusion == "success" {
						icon = tui.SuccessStyle.Render("✓")
					} else {
						icon = tui.ErrorStyle.Render("✗")
					}
				}
				created := r.CreatedAt
				if len(created) >= 10 {
					created = created[:10]
				}
				b.WriteString(fmt.Sprintf("  %s %-20s %-8s %-10s %s\n",
					icon, r.DisplayTitle, r.HeadBranch, r.Event, tui.HelpStyle.Render(created)))
			}
		}

		b.WriteString(tui.HelpStyle.Render("\nesc: back  enter: menu  q: quit"))
	}

	return b.String()
}

func renderField(label, value string) string {
	return tui.LabelStyle.Render(label+":") + " " + tui.ValueStyle.Render(value) + "\n"
}
