package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

type appModel struct {
	width, height int
	xPos, yPos    int
	xVel, yVel    int
	boxWidth      int
	boxHeight     int
}

type tickMsg time.Time

func InitialAppModel() *appModel {
	return &appModel{
		width:     0,
		height:    0,
		xPos:      0,
		yPos:      0,
		xVel:      1,
		yVel:      1,
		boxWidth:  10,
		boxHeight: 5,
	}
}

func (m appModel) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		tea.Tick(time.Second/30, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}),
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
	case tickMsg:
		// Update position
		m.xPos += m.xVel
		m.yPos += m.yVel

		// Bounce off walls
		if m.xPos <= 0 || m.xPos+m.boxWidth >= m.width {
			m.xVel = -m.xVel
		}
		if m.yPos <= 0 || m.yPos+m.boxHeight >= m.height {
			m.yVel = -m.yVel
		}

		// Keep within bounds
		if m.xPos < 0 {
			m.xPos = 0
		}
		if m.xPos+m.boxWidth > m.width {
			m.xPos = m.width - m.boxWidth
		}
		if m.yPos < 0 {
			m.yPos = 0
		}
		if m.yPos+m.boxHeight > m.height {
			m.yPos = m.height - m.boxHeight
		}

		return m, tea.Tick(time.Second/30, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	}

	return m, nil
}

func (m appModel) View() tea.View {

	box := lipgloss.NewStyle().Width(10).Height(5).Border(lipgloss.NormalBorder()).AlignHorizontal(0.5).AlignVertical(0.5)

	a := lipgloss.NewLayer(box.Render())

	canvas := lipgloss.NewCanvas(
		a.X(m.xPos).Y(m.yPos),
	)

	// canvas := lipgloss.NewCanvas()

	v := tea.NewView("")
	v.BackgroundColor = lipgloss.Black
	v.AltScreen = true
	v.WindowTitle = "wingman"
	v.SetContent(canvas.Render())

	return v
}
