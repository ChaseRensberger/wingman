package ui

import (
	"github.com/charmbracelet/lipgloss"
)

type Styles struct {
	User        lipgloss.Style
	Assistant   lipgloss.Style
	Tool        lipgloss.Style
	ToolBox     lipgloss.Style
	ToolSection lipgloss.Style
	Error       lipgloss.Style
	Success     lipgloss.Style
	Running     lipgloss.Style
	Code        lipgloss.Style
	Header      lipgloss.Style
	Subtle      lipgloss.Style
	Border      lipgloss.Style
	StatusBar   lipgloss.Style
}

func DefaultStyles() Styles {
	return Styles{
		User: lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true).
			MarginTop(1).
			MarginBottom(0),

		Assistant: lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			MarginTop(0).
			MarginBottom(1),

		Tool: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true).
			MarginTop(1),

		ToolBox: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			MarginTop(0).
			MarginBottom(1),

		ToolSection: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Bold(true).
			MarginTop(0),

		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true).
			MarginTop(1),

		Success: lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true),

		Running: lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true),

		Code: lipgloss.NewStyle().
			Foreground(lipgloss.Color("14")).
			Background(lipgloss.Color("236")).
			Padding(0, 1),

		Header: lipgloss.NewStyle().
			Foreground(lipgloss.Color("13")).
			Bold(true).
			MarginTop(1).
			MarginBottom(1),

		Subtle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),

		Border: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1),

		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Background(lipgloss.Color("235")).
			Padding(0, 1),
	}
}
