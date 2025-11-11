package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load(".env.local")

	provider, err := CreateInferenceProvider("anthropic", map[string]any{
		"api_key":     "",
		"model":       "",
		"max_tokens":  4096,
		"temperature": 1.0,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create provider: %v\n", err)
		os.Exit(1)
	}

	session := CreateSession(provider)

	scanner := bufio.NewScanner(os.Stdin)
	ctx := context.Background()

	var messages []AnthropicMessage

	fmt.Println("Type your message (Ctrl+C to exit):")

	for scanner.Scan() {
		userInput := scanner.Text()
		if userInput == "" {
			continue
		}

		messages = append(messages, AnthropicMessage{
			Role:    "user",
			Content: userInput,
		})

		result, err := session.RunInference(ctx, messages)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		resp, ok := result.(*AnthropicMessageResponse)
		if !ok {
			fmt.Fprintf(os.Stderr, "Unexpected response type\n")
			continue
		}

		if len(resp.Content) > 0 {
			assistantText := resp.Content[0].Text
			fmt.Println("\nAssistant:", assistantText)

			messages = append(messages, AnthropicMessage{
				Role:    "assistant",
				Content: assistantText,
			})
		}

		fmt.Println("\nYour message:")
	}
}
