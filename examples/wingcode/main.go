package main

import (
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"

	"wingman/agent"
	"wingman/provider/anthropic"
	"wingman/session"
	"wingman/tool"
	"wingman/ui"
)

func main() {
	godotenv.Load(".env.local")

	workDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	p := anthropic.New(anthropic.Config{})
	if p == nil {
		log.Fatal("ANTHROPIC_API_KEY not set")
	}

	a := agent.New("WingcodeAgent",
		agent.WithInstructions("You are Wingcode, a helpful coding assistant. You help users with programming tasks, code review, and technical questions. Be concise but thorough."),
		agent.WithMaxTokens(4096),
		agent.WithTools(
			tool.NewBashTool(workDir),
			tool.NewReadTool(workDir),
			tool.NewWriteTool(workDir),
			tool.NewEditTool(workDir),
			tool.NewGlobTool(workDir),
			tool.NewGrepTool(workDir),
		),
	)

	s := session.New(
		session.WithAgent(a),
		session.WithProvider(p),
	)

	fmt.Printf("Starting Wingcode Interactive Session...\n")
	fmt.Printf("Session ID: %s\n\n", s.ID())
	fmt.Printf("Type /help for available commands.\n")
	fmt.Printf("Press Ctrl+C to exit.\n\n")

	m := ui.New(s, p, a)

	program := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := program.Run(); err != nil {
		log.Fatal(err)
	}
}
