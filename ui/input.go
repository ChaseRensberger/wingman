package ui

import (
	"charm.land/bubbletea/v2"
)

type InputComponent interface {
	Component
}

type inputModel struct {
	value   string
	cursor  int
	focused bool
	width   int
	height  int
}

func CreateInput() InputComponent {
	return &inputModel{
		focused: true,
		width:   80,
		height:  3,
	}
}

func (m *inputModel) Init() tea.Cmd {
	return nil
}
func (m *inputModel) Update(msg tea.Msg) (Component, tea.Cmd) {
	if !m.focused {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Key().Code {
		case tea.KeyEnter:
			m.value = ""
			m.cursor = 0
		case tea.KeyBackspace:
			if m.cursor > 0 {
				m.value = m.value[:m.cursor-1] + m.value[m.cursor:]
				m.cursor--
			}
		case tea.KeyLeft:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyRight:
			if m.cursor < len(m.value) {
				m.cursor++
			}
		default:
			key := msg.Key()
			if key.Text != "" && key.Mod == 0 {
				m.value = m.value[:m.cursor] + key.Text + m.value[m.cursor:]
				m.cursor += len(key.Text)
			}
		}
	}
	return m, nil
}

func (m *inputModel) View() string {
	if !m.focused {
		return m.value
	}

	var display string
	if m.cursor < len(m.value) {
		display = m.value[:m.cursor] + "█" + m.value[m.cursor+1:]
	} else {
		display = m.value + "█"
	}
	return display
}
