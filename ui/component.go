package ui

import (
	tea "charm.land/bubbletea/v2"
)

type Component interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Component, tea.Cmd)
	View() string
}
