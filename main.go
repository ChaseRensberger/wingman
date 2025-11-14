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
	var messages []AnthropicMessage

	ctx := context.Background()
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("\n> ")

	for scanner.Scan() {
		userInput := scanner.Text()
		if userInput == "" {
			fmt.Print("\n> ")
			continue
		}

		printBox(userInput, true)

		messages = append(messages, AnthropicMessage{
			Role:    "user",
			Content: userInput,
		})

		result, err := session.RunInference(ctx, messages)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			fmt.Print("\n> ")
			continue
		}

		resp, ok := result.(*AnthropicMessageResponse)
		if !ok {
			fmt.Fprintf(os.Stderr, "Unexpected response type\n")
			fmt.Print("\n> ")
			continue
		}

		if len(resp.Content) > 0 {
			assistantText := resp.Content[0].Text
			printBox(assistantText, false)

			messages = append(messages, AnthropicMessage{
				Role:    "assistant",
				Content: assistantText,
			})
		}

		fmt.Print("\n> ")
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
	}
}
