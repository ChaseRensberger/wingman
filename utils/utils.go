package utils

import (
	"github.com/charmbracelet/lipgloss/v2"
)

var userColor = lipgloss.Blue
var agentColor = lipgloss.Red
var toolColor = lipgloss.Color("#FC9003")

var borderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
var userStyle = lipgloss.NewStyle().Foreground(userColor)

func UserPrint(text string) {
	lipgloss.Println(borderStyle.BorderForeground(userColor).Render(userStyle.Bold(true).Render("User: ") + userStyle.Render(text)))
}

var agentStyle = lipgloss.NewStyle().Foreground(agentColor)

func AgentPrint(text string) {
	lipgloss.Println(borderStyle.BorderForeground(agentColor).Render(agentStyle.Bold(true).Render("Agent: ") + agentStyle.Render(text)))
}

var toolStyle = lipgloss.NewStyle().Foreground(toolColor)

func ToolPrint(text string) {
	lipgloss.Println(borderStyle.BorderForeground(toolColor).Render(toolStyle.Render(text)))
}
