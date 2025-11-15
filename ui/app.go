package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

type model struct {
	input  string
	cursor int
}

func InitialModel() *model {
	return &model{
		input:  "",
		cursor: 0,
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
		case tea.KeyEnter:
			m.input = ""
			m.cursor = 0
		case tea.KeyBackspace:
			if m.cursor > 0 {
				m.input = m.input[:m.cursor-1] + m.input[m.cursor:]
				m.cursor--
			}
		case tea.KeyLeft:
			if m.cursor > 0 {
				m.cursor--
			}

		case tea.KeyRight:
			if m.cursor < len(m.input) {
				m.cursor++
			}
		default:
			key := msg.Key()
			if key.Text != "" && key.Mod == 0 {
				m.input = m.input[:m.cursor] + key.Text + m.input[m.cursor:]
				m.cursor += len(key.Text)
			}
		}

	}
	return m, nil
}

func (m *model) View() tea.View {
	var display string
	if m.cursor < len(m.input) {
		display = m.input[:m.cursor] + "█" + m.input[m.cursor+1:]
	} else {
		display = m.input + "█"
	}
	s := fmt.Sprintf(
		"Type a message:\n%s\n\n"+
			"Enter to send, Esc to quit",
		display,
	)
	view := tea.NewView(s)
	view.AltScreen = true
	return view
}
