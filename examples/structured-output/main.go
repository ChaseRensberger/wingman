package main

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"wingman/agent"
	"wingman/models"
	"wingman/utils"
)

func main() {
	godotenv.Load(".env.local")

	schema := map[string]any{
		"name": "person_info",
		"description": "Information about a person including their name and occupation",
		"strict": true,
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type": "string",
					"description": "The person's full name",
				},
				"occupation": map[string]any{
					"type": "string",
					"description": "The person's occupation or job title",
				},
				"location": map[string]any{
					"type": "string",
					"description": "Where the person lives or works",
				},
			},
			"required": []string{"name", "occupation", "location"},
			"additionalProperties": false,
		},
	}

	agent := agent.CreateAgent("structured-wingman").
		WithProvider("anthropic").
		WithInstructions("You are a helpful assistant that extracts structured information.").
		WithStructuredOutput(schema).
		WithConfig(map[string]any{
			"model": "claude-sonnet-4-5-20251022",
			"max_tokens":  2048,
			"temperature": 1.0,
		})

	ctx := context.Background()
	userMessage := "Tell me about Albert Einstein"

	messages := []models.WingmanMessage{
		{
			Role:    "user",
			Content: userMessage,
		},
	}

	result, err := agent.RunInference(ctx, messages)
	if err != nil {
		log.Fatal(err)
	}

	utils.UserPrint(userMessage)
	if len(result.Content) > 0 {
		utils.AgentPrint(result.Content[0].Text)
	}

	utils.ToolPrint(fmt.Sprintf("Tokens used - Input: %d, Output: %d", result.Usage.InputTokens, result.Usage.OutputTokens))
}
