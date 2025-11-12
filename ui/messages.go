package ui

import (
	"github.com/charmbracelet/lipgloss/v2"
)

type messagesModel struct {
	// state
	width, height int
	title         string
}

func initialMessagesModel() *messagesModel {
	return &messagesModel{
		title: "Messages",
		width: 0,
	}
}

func (m messagesModel) view() *lipgloss.Layer {
	content := lipgloss.NewStyle().
		Width(m.width).
		Padding(1).
		Border(lipgloss.NormalBorder()).
		Render(m.title)

	return lipgloss.NewLayer(content)
}
