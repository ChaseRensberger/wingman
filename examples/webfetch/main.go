package main

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"wingman/agent"
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
		agent.WithProvider(p),
		agent.WithTools(
			tool.NewWebFetchTool(),
		),
	)

	s := session.New(
		session.WithAgent(a),
	)

	ctx := context.Background()
	message := "Fetch https://news.ycombinator.com and tell me what the top 3 stories are about"

	fmt.Printf("User: %s\n\n", message)

	result, err := s.Run(ctx, message)
	if err != nil {
		log.Fatal(err)
	}

	for _, tc := range result.ToolCalls {
		if tc.Error != nil {
			fmt.Printf("Tool: [%s] Error: %v\n", tc.ToolName, tc.Error)
		} else {
			fmt.Printf("Tool: [%s] Fetched %d bytes\n", tc.ToolName, len(tc.Output))
		}
	}

	fmt.Println()
	fmt.Printf("Agent: %s\n", result.Response)
	fmt.Println()
	fmt.Printf("Steps: %d | Tokens - Input: %d, Output: %d\n",
		result.Steps, result.Usage.InputTokens, result.Usage.OutputTokens)
}
