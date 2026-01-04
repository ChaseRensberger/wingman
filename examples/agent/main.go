package main

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"wingman/agent"
	"wingman/models"
	"wingman/provider/anthropic"
	"wingman/utils"
)

func main() {
	godotenv.Load(".env.local")

	agent, err := agent.CreateAgent("WingmanAgent",
		agent.WithProvider(anthropic.New(anthropic.Config{})),
		agent.WithInstructions("You are a helpful assistant that speaks like a pirate."),
		agent.WithMaxTokens(2048),
		agent.WithTemperature(1.0),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	userMessage := "What is the weather like in San Diego?"

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
