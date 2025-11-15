package ui

import (
	tea "charm.land/bubbletea/v2"
)

type Component interface {
	Init() tea.Cmd
	Update() (tea.Model, tea.Cmd)
	View() string
}
