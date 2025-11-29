package main

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"wingman/agent"
	"wingman/models"
)

func main() {
	godotenv.Load(".env.local")

	agent := agent.NewAgent("wingman").
		WithProvider("anthropic").
		WithConfig(map[string]any{
			"max_tokens":  2048,
			"temperature": 1.0,
		})

	ctx := context.Background()

	messages := []models.WingmanMessage{
		{
			Role:    "user",
			Content: "Hello! What is the capital of France?",
		},
	}

	result, err := agent.RunInference(ctx, messages)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\nRESPONSE 1:")
	fmt.Println("=========================")
	if len(result.Content) > 0 {
		fmt.Println(result.Content[0].Text)
	}
	fmt.Printf("Tokens used - Input: %d, Output: %d\n", result.Usage.InputTokens, result.Usage.OutputTokens)
}
