package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

var primaryColor = lipgloss.NewStyle().Foreground(lipgloss.White).Background(lipgloss.Black).Padding(2, 2)

type model struct {
	canvas        *lipgloss.Canvas
	width, height int
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	}

	return m, nil
}

func (m model) View() string {
	return ""
}
