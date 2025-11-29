package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"os"
	"strings"
	"wingman/agent"
	"wingman/models"
)

func main() {
	godotenv.Load(".env.local")

	chatAgent := agent.CreateAgent("chat").
		WithProvider("anthropic").
		WithConfig(map[string]any{
			"max_tokens":  2048,
			"temperature": 0.7,
		})

	ctx := context.Background()
	conversationHistory := []models.WingmanMessage{}

	fmt.Println("Welcome to Wingman Chat!")
	fmt.Println("Type 'exit' to quit, 'clear' to clear history")
	fmt.Println("=================================================")

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("You: ")
		userInput, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		userInput = strings.TrimSpace(userInput)

		if userInput == "" {
			continue
		}

		if strings.ToLower(userInput) == "exit" {
			fmt.Println("Goodbye!")
			break
		}

		if strings.ToLower(userInput) == "clear" {
			conversationHistory = []models.WingmanMessage{}
			fmt.Println("Conversation history cleared.")
			continue
		}

		conversationHistory = append(conversationHistory, models.WingmanMessage{
			Role:    "user",
			Content: userInput,
		})

		result, err := chatAgent.RunInference(ctx, conversationHistory)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			conversationHistory = conversationHistory[:len(conversationHistory)-1]
			continue
		}

		var assistantContent string
		if len(result.Content) > 0 {
			assistantContent = result.Content[0].Text
		}

		conversationHistory = append(conversationHistory, models.WingmanMessage{
			Role:    "assistant",
			Content: assistantContent,
		})

		fmt.Printf("\nAssistant: %s\n", assistantContent)
		fmt.Printf("(Tokens - Input: %d, Output: %d)\n\n", result.Usage.InputTokens, result.Usage.OutputTokens)
	}
}
