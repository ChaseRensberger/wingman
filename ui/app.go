package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

type appModel struct {
	// components
	messagesComponent *messagesModel

	// state
	width, height    int
	focusedComponent string
}

func InitialAppModel() appModel {
	return appModel{
		width:             0,
		height:            0,
		messagesComponent: initialMessagesModel(),
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

	messagesLayer := m.messagesComponent.view()

	canvas := lipgloss.NewCanvas(
		messagesLayer,
	)

	v := tea.NewView("")
	v.BackgroundColor = lipgloss.Black
	v.AltScreen = true
	v.WindowTitle = "wingman"
	v.SetContent(canvas.Render())

	return v
}
