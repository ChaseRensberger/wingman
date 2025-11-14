package main

import (
	"os"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/charmbracelet/x/term"
)

func printBox(text string, isUser bool) {
	width, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil {
		width = 80
	}

	style := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Width(width)

	if isUser {
		style = style.BorderForeground(lipgloss.Color("12")) // Blue
	} else {
		style = style.BorderForeground(lipgloss.Color("9")) // Red
	}

	rendered := style.Render(text)
	println(rendered)
}
