package tui

import "github.com/charmbracelet/lipgloss"

var (
	ProfileBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1).
			MarginBottom(1)

	MessageStranger = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			PaddingLeft(2)

	MessageYou = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			PaddingLeft(2)

	StatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	ErrorText = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	ProgressBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205"))

	Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("63")).
		Bold(true).
		MarginBottom(1)

	Subtle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	CommandStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))
)
