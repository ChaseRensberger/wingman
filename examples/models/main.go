package main

import (
	"context"
	"fmt"
	"log"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/providers"
)

func main() {
	client := provider.NewClient(nil)

	request := models.Request{
		Model:      models.ModelRef{Provider: "opencode", ID: "claude-sonnet-4-6"},
		System:     "You are concise.",
		Messages:   []models.Message{models.NewUserText("Say hello in one short sentence.")},
		Generation: models.Generation{MaxTokens: 40},
	}

	response, err := client.Generate(context.Background(), request)
	if err != nil {
		log.Fatal(err)
	}

	for _, part := range response.Content {
		if text, ok := part.(models.TextPart); ok {
			fmt.Println(text.Text)
		}
	}
}
