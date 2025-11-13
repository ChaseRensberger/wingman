package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

type appModel struct {
	width, height int
	view          tea.View
}

func InitialAppModel() *appModel {
	v := tea.NewView("")
	v.BackgroundColor = lipgloss.Black
	v.AltScreen = true
	v.WindowTitle = "wingman"

	return &appModel{
		width:  0,
		height: 0,
		view:   v,
	}
}

func (m appModel) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
	)
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	}

	return m, cmd
}

func (m appModel) View() tea.View {
	canvas := lipgloss.NewCanvas()
	m.view.SetContent(canvas.Render())
	return m.view
}
