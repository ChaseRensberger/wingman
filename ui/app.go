package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
)

type model struct {
	count int
}

func InitialModel() *model {
	return &model{
		count: 0,
	}
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Key().Code {
		case tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyUp:
			m.count++
		case tea.KeyDown:
			m.count--
		}

	}
	return m, nil
}

func (m *model) View() tea.View {
	view := tea.NewView(strconv.Itoa(m.count) + "\n\nPress Esc to quit")
	view.AltScreen = true
	return view
}
