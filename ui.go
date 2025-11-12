package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

type mainModel struct{}

type appModel struct {
	canvas        *lipgloss.Canvas
	width, height int

	main *mainModel
}

func initialModel() appModel {
	return appModel{
		canvas: lipgloss.NewCanvas(),
		width:  0,
		height: 0,
	}
}

func (m appModel) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
	)
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}

	}

	return m, nil
}

func (m appModel) View() tea.View {

	// mainLayer :=

	v := tea.NewView("wingman")
	v.BackgroundColor = lipgloss.Black
	v.AltScreen = true
	v.WindowTitle = "wingman"
	return v
}
