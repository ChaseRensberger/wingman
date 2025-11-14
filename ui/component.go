package ui

import (
	tea "charm.land/bubbletea/v2"
)

type Component interface {
	Init()
	Update(tea.Msg)
	View() string
}

type Sizeable interface {
	SetSize(width, height int) tea.Cmd
	GetSize() (int, int)
}

type Focusable interface {
	Focus() tea.Cmd
	Blur() tea.Cmd
	IsFocused() bool
}

type Positional interface {
	SetPosition(x, y int) tea.Cmd
}
