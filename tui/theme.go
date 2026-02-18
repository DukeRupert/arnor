package tui

import "github.com/charmbracelet/lipgloss"

var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("63")).
			MarginBottom(1)

	LabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Width(16)

	ValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	SuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	SpinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("63"))
)
