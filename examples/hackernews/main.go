package main

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"github.com/chaserensberger/wingman/wingagent/session"
	"github.com/chaserensberger/wingman/wingagent/tool"
	"github.com/chaserensberger/wingman/wingmodels/providers/anthropic"
)

func main() {
	godotenv.Load(".env.local")

	p, err := anthropic.New()
	if err != nil {
		log.Fatalf("failed to create Anthropic provider: %v", err)
	}

	s := session.New(
		session.WithModel(p),
		session.WithSystem("Your job is to read the top 5 posts on hackernews and structure them as json. Return ONLY a JSON array of {name, link, points} objects."),
		session.WithTools(tool.NewWebFetchTool()),
	)

	result, err := s.Run(context.Background(), "Fetch the top 5 posts on hackernews for me")
	if err != nil {
		log.Fatal(err)
	}

	for _, tc := range result.ToolCalls {
		if tc.Error != "" {
			fmt.Printf("Tool: [%s] Error: %s\n", tc.ToolName, tc.Error)
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
