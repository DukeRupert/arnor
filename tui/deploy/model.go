package deploy

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
	phaseSelectEnv
	phaseConfirm
	phaseRunning
	phaseDone
)

type triggerDoneMsg struct {
	err error
}

// Model is the BubbleTea model for the deploy screen.
type Model struct {
	phase phase

	projects []config.Project
	cursor   int
	store    config.Store

	selectedProject config.Project
	envNames        []string
	envCursor       int
	selectedEnv     string

	spinner spinner.Model
	err     error
}

// New creates a new deploy model.
func New(projects []config.Project, store config.Store) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = tui.SpinnerStyle

	return Model{
		phase:    phaseSelectProject,
		projects: projects,
		store:    store,
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
	case phaseSelectEnv:
		return m.updateSelectEnv(msg)
	case phaseConfirm:
		return m.updateConfirm(msg)
	case phaseRunning:
		return m.updateRunning(msg)
	case phaseDone:
		return m.updateDone(msg)
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
			m.envNames = nil
			for name := range m.selectedProject.Environments {
				m.envNames = append(m.envNames, name)
			}
			if len(m.envNames) == 0 {
				m.err = fmt.Errorf("project %s has no environments configured", m.selectedProject.Name)
				m.phase = phaseDone
				return m, nil
			}
			if len(m.envNames) == 1 {
				m.selectedEnv = m.envNames[0]
				m.phase = phaseConfirm
				return m, nil
			}
			m.envCursor = 0
			m.phase = phaseSelectEnv
			return m, nil
		case "esc":
			return m, func() tea.Msg {
				return tui.SwitchScreenMsg{Screen: tui.ScreenMenu}
			}
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
			if m.envCursor < len(m.envNames)-1 {
				m.envCursor++
			}
		case "enter":
			m.selectedEnv = m.envNames[m.envCursor]
			m.phase = phaseConfirm
			return m, nil
		case "esc":
			m.phase = phaseSelectProject
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter", "y":
			m.phase = phaseRunning
			return m, tea.Batch(m.spinner.Tick, m.triggerDeploy())
		case "esc", "n":
			if len(m.envNames) > 1 {
				m.phase = phaseSelectEnv
			} else {
				m.phase = phaseSelectProject
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateRunning(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case triggerDoneMsg:
		m.err = msg.err
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

func (m Model) triggerDeploy() tea.Cmd {
	repo := m.selectedProject.Repo
	projectName := m.selectedProject.Name
	envName := m.selectedEnv
	env := m.selectedProject.Environments[envName]
	workflowFile := project.WorkflowFile(envName)
	ref := project.DeployRef(env)
	s := m.store

	return func() tea.Msg {
		dockerHubUsername, err := s.GetCredential("dockerhub", "default", "username")
		if err != nil {
			return triggerDoneMsg{err: fmt.Errorf("dockerhub username: %w", err)}
		}
		if err := project.EnsureWorkflowDispatch(repo, envName, projectName, dockerHubUsername); err != nil {
			return triggerDoneMsg{err: err}
		}
		err = project.TriggerWorkflow(repo, workflowFile, ref)
		return triggerDoneMsg{err: err}
	}
}

// View renders the current phase.
func (m Model) View() string {
	var b strings.Builder

	b.WriteString(tui.TitleStyle.Render("Deploy"))
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

	case phaseSelectEnv:
		b.WriteString(renderField("Project", m.selectedProject.Name))
		b.WriteString("\nSelect environment:\n\n")
		for i, name := range m.envNames {
			line := fmt.Sprintf("  %s", name)
			if i == m.envCursor {
				line = tui.CursorStyle.Render("> " + name)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString(tui.HelpStyle.Render("\nj/k: navigate  enter: select  esc: back"))

	case phaseConfirm:
		env := m.selectedProject.Environments[m.selectedEnv]
		b.WriteString(renderField("Project", m.selectedProject.Name))
		b.WriteString(renderField("Repo", m.selectedProject.Repo))
		b.WriteString(renderField("Environment", m.selectedEnv))
		b.WriteString(renderField("Branch", env.Branch))
		b.WriteString(renderField("Workflow", project.WorkflowFile(m.selectedEnv)))
		b.WriteString("\nTrigger this deploy?")
		b.WriteString(tui.HelpStyle.Render("\nenter/y: deploy  esc/n: back"))

	case phaseRunning:
		b.WriteString(renderField("Project", m.selectedProject.Name))
		b.WriteString(renderField("Environment", m.selectedEnv))
		b.WriteString("\n")
		b.WriteString(m.spinner.View())
		b.WriteString(" Dispatching workflow...")

	case phaseDone:
		b.WriteString(renderField("Project", m.selectedProject.Name))
		b.WriteString(renderField("Environment", m.selectedEnv))
		b.WriteString("\n")
		if m.err != nil {
			b.WriteString(tui.ErrorStyle.Render("Error: "))
			b.WriteString(m.err.Error())
		} else {
			b.WriteString(tui.SuccessStyle.Render("Workflow dispatched successfully!"))
		}
		b.WriteString(tui.HelpStyle.Render("\nenter: menu  q: quit"))
	}

	return b.String()
}

func renderField(label, value string) string {
	return tui.LabelStyle.Render(label+":") + " " + tui.ValueStyle.Render(value) + "\n"
}
