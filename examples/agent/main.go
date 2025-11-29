package main

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"wingman/agent"
	"wingman/models"
	"wingman/utils"
)

func main() {
	godotenv.Load(".env.local")

	agent := agent.CreateAgent("wingman").
		WithProvider("anthropic").
		WithInstructions("You are a helpful assistant that speaks like a pirate.").
		WithConfig(map[string]any{
			"max_tokens":  2048,
			"temperature": 1.0,
		})

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
