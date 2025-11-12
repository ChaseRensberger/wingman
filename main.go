package main

import (
	// "bufio"
	// "context"
	// "fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/joho/godotenv"
	"wingman/ui"
)

func main() {
	godotenv.Load(".env.local")

	// provider, err := CreateInferenceProvider("anthropic", map[string]any{
	// 	"api_key":     "",
	// 	"model":       "",
	// 	"max_tokens":  4096,
	// 	"temperature": 1.0,
	// })
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "Failed to create provider: %v\n", err)
	// 	os.Exit(1)
	// }

	// session := CreateSession(provider)
	// var messages []AnthropicMessage

	// messages = append(messages, AnthropicMessage{
	// 	Role:    "user",
	// 	Content: userInput,
	// })
	// result, err := session.RunInference(ctx, messages)
	// resp, ok := result.(*AnthropicMessageResponse)
	// if !ok {
	// 	fmt.Fprintf(os.Stderr, "Unexpected response type\n")
	// 	continue
	// }
	// if len(resp.Content) > 0 {
	// 	assistantText := resp.Content[0].Text
	// 	fmt.Println("\nAssistant:", assistantText)
	//
	// 	messages = append(messages, AnthropicMessage{
	// 		Role:    "assistant",
	// 		Content: assistantText,
	// 	})
	// }

	if _, err := tea.NewProgram(ui.InitialAppModel()).Run(); err != nil {
		os.Exit(1)
	}

}
