package main

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"wingman/agent"
	"wingman/internal/utils"
	"wingman/provider/anthropic"
	"wingman/session"
	"wingman/tool"
)

func main() {
	godotenv.Load(".env.local")

	p := anthropic.New(anthropic.Config{})
	if p == nil {
		log.Fatal("ANTHROPIC_API_KEY not set")
	}

	a := agent.New("WebResearcher",
		agent.WithInstructions("You are a helpful research assistant. Use the webfetch tool to retrieve information from websites when needed. Summarize the key points clearly and concisely."),
		agent.WithMaxTokens(4096),
		agent.WithTools(
			tool.NewWebFetchTool(),
		),
	)

	s := session.New(
		session.WithAgent(&a),
		session.WithProvider(p),
	)

	ctx := context.Background()
	message := "Fetch https://news.ycombinator.com and tell me what the top 3 stories are about"

	utils.UserPrint(message)
	fmt.Println()

	result, err := s.Run(ctx, message)
	if err != nil {
		log.Fatal(err)
	}

	for _, tc := range result.ToolCalls {
		if tc.Error != nil {
			utils.ToolPrint(fmt.Sprintf("[%s] Error: %v", tc.ToolName, tc.Error))
		} else {
			utils.ToolPrint(fmt.Sprintf("[%s] Fetched %d bytes", tc.ToolName, len(tc.Output)))
		}
	}

	fmt.Println()
	utils.AgentPrint(result.Response)
	fmt.Println()
	utils.ToolPrint(fmt.Sprintf("Steps: %d | Tokens - Input: %d, Output: %d",
		result.Steps, result.Usage.InputTokens, result.Usage.OutputTokens))
}
