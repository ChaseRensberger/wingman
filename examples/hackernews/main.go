package main

import (
	"context"
	"fmt"
	"log"
	"github.com/chaserensberger/wingman/agent"
	"github.com/chaserensberger/wingman/provider/anthropic"
	"github.com/chaserensberger/wingman/session"
	"github.com/chaserensberger/wingman/tool"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load(".env.local")

	schema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "The name of the post",
				},
				"link": map[string]any{
					"type":        "string",
					"description": "The URL of the post",
				},
				"points": map[string]any{
					"type":        "integer",
					"description": "The number of points of the post",
				},
			},
			"required":             []string{"name", "link", "points"},
			"additionalProperties": false,
		},
	}

	p := anthropic.New()

	a := agent.New("Hackernews Parser",
		agent.WithInstructions("Your job is to read the top 5 posts on hackernews and structure them as json them as json"),
		agent.WithProvider(p),
		agent.WithOutputSchema(schema),
		agent.WithTools(
			tool.NewWebFetchTool(),
		),
	)

	s := session.New(session.WithAgent(a))
	result, err := s.Run(context.Background(), "Fetch the top 5 posts on hackernews for me")
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
