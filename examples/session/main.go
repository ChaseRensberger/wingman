package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"

	"wingman/agent"
	"wingman/provider/anthropic"
	"wingman/session"
	"wingman/tool"
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

	a := agent.New("WingmanAgent",
		agent.WithInstructions("You are a helpful coding assistant. Keep track of our conversation."),
		agent.WithProvider(p),
		agent.WithTools(
			tool.NewBashTool(),
			tool.NewReadTool(),
		),
	)

	s := session.New(
		session.WithWorkDir(workDir),
		session.WithAgent(a),
	)

	ctx := context.Background()

	fmt.Printf("Session ID: %s\n\n", s.ID())

	messages := []string{
		"What is 2 + 2?",
		"What did I just ask you?",
		"Now multiply that result by 10",
	}

	for _, message := range messages {
		fmt.Printf("User: %s\n\n", message)

		result, err := s.Run(ctx, message)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Agent: %s\n", result.Response)
		fmt.Println()
		fmt.Printf("Tokens - Input: %d, Output: %d\n", result.Usage.InputTokens, result.Usage.OutputTokens)
		fmt.Println()
	}

	fmt.Printf("Total messages in session: %d\n", len(s.History()))
}
