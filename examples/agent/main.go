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

	agent, err := agent.CreateAgent("wingman",
		agent.WithProvider("anthropic"),
		agent.WithInstructions("You are a helpful assistant that speaks like a pirate."),
		agent.WithConfig(map[string]any{
			"max_tokens":  2048,
			"temperature": 1.0,
			"thinking": map[string]any{
				"type":          "enabled",
				"budget_tokens": 10000,
			},
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	userMessage := "What is the capital of the United States?"

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
