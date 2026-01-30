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
	"wingman/utils"
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
		agent.WithMaxTokens(4096),
		agent.WithTools(
			tool.NewBashTool(workDir),
			tool.NewReadTool(workDir),
		),
	)

	s := session.New(
		session.WithAgent(a),
		session.WithProvider(p),
	)

	ctx := context.Background()

	fmt.Printf("Session ID: %s\n\n", s.ID())

	prompts := []string{
		"What is 2 + 2?",
		"What did I just ask you?",
		"Now multiply that result by 10",
	}

	for _, prompt := range prompts {
		utils.UserPrint(prompt)
		fmt.Println()

		result, err := s.Run(ctx, prompt)
		if err != nil {
			log.Fatal(err)
		}

		utils.AgentPrint(result.Response)
		fmt.Println()
		utils.ToolPrint(fmt.Sprintf("Tokens - Input: %d, Output: %d",
			result.Usage.InputTokens, result.Usage.OutputTokens))
		fmt.Println()
	}

	fmt.Printf("Total messages in session: %d\n", len(s.History()))
}
