package main

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"wingman/agent"
	"wingman/provider/anthropic"
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

	messages := []anthropic.AnthropicMessage{
		{
			Role:    "user",
			Content: "Hello! What is the capital of France?",
		},
	}

	result, err := agent.RunInference(ctx, messages)
	if err != nil {
		log.Fatal(err)
	}

	response, ok := result.(*anthropic.AnthropicMessageResponse)
	if !ok {
		log.Fatal("failed to cast response")
	}

	fmt.Println("\nRESPONSE 1:")
	fmt.Println("=========================")
	if len(response.Content) > 0 {
		fmt.Println(response.Content[0].Text)
	}
	fmt.Printf("Tokens used - Input: %d, Output: %d\n", response.Usage.InputTokens, response.Usage.OutputTokens)
}
