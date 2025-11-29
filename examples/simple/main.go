package main

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"wingman/models"
	"wingman/provider"
	"wingman/session"
)

func main() {
	godotenv.Load(".env.local")

	anthropicClient, err := provider.GetProviderFromRegistry("anthropic", map[string]any{})
	if err != nil {
		log.Fatal(err)
	}

	anthropicSession := session.CreateSession(anthropicClient)

	ctx := context.Background()

	message := []models.WingmanMessage{
		{
			Role:    "user",
			Content: "Hello! What is the capital of France?",
		},
	}

	result, err := anthropicSession.RunInference(ctx, message)
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
