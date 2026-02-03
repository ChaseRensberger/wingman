package main

import (
	"context"
	"fmt"
	"log"
	"wingman/agent"
	"wingman/internal/utils"
	"wingman/provider/anthropic"
	"wingman/session"
	"wingman/tool"

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
		agent.WithOutputSchema(schema),
		agent.WithTools(
			tool.NewWebFetchTool(),
		),
	)

	s := session.New(session.WithAgent(a), session.WithProvider(p))
	result, err := s.Run(context.Background(), "Fetch the top 5 posts on hackernews for me")
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
