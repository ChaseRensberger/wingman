package main

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"wingman/provider"
	"wingman/provider/anthropic"
	"wingman/provider/registry"
	"wingman/session"
)

func main() {
	godotenv.Load(".env.local")
	fmt.Println("SUPPORTED PROVIDERS")
	fmt.Println("=========================")
	providers := registry.ListProvidersInRegistry()
	for _, name := range providers {
		fmt.Println(name)
	}
	fmt.Println("=========================")

	anthropicBuilder, err := registry.GetBuilder("anthropic")
	if err != nil {
		log.Fatal(err)
	}
	anthropicClient, err := anthropicBuilder(make(map[string]any))
	if err != nil {
		log.Fatal(err)
	}

	inferenceProvider, ok := anthropicClient.(provider.InferenceProvider)
	if !ok {
		log.Fatal("failed to cast to InferenceProvider")
	}

	basicSession := session.CreateSession(inferenceProvider)

	ctx := context.Background()

	messages1 := []anthropic.AnthropicMessage{
		{
			Role:    "user",
			Content: "Hello! What is the capital of France?",
		},
	}

	result1, err := basicSession.RunInference(ctx, messages1)
	if err != nil {
		log.Fatal(err)
	}

	response1, ok := result1.(*anthropic.AnthropicMessageResponse)
	if !ok {
		log.Fatal("failed to cast response")
	}

	fmt.Println("\nRESPONSE 1:")
	fmt.Println("=========================")
	if len(response1.Content) > 0 {
		fmt.Println(response1.Content[0].Text)
	}
	fmt.Printf("Tokens used - Input: %d, Output: %d\n", response1.Usage.InputTokens, response1.Usage.OutputTokens)

	messages2 := []anthropic.AnthropicMessage{
		{
			Role:    "user",
			Content: "What is its population?",
		},
	}

	result2, err := basicSession.RunInference(ctx, messages2)
	if err != nil {
		log.Fatal(err)
	}

	response2, ok := result2.(*anthropic.AnthropicMessageResponse)
	if !ok {
		log.Fatal("failed to cast response")
	}

	fmt.Println("\nRESPONSE 2:")
	fmt.Println("=========================")
	if len(response2.Content) > 0 {
		fmt.Println(response2.Content[0].Text)
	}
	fmt.Printf("Tokens used - Input: %d, Output: %d\n", response2.Usage.InputTokens, response2.Usage.OutputTokens)
}
