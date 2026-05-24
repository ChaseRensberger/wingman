package main

import (
	"context"
	"fmt"
	"log"
	"math"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
	"github.com/chaserensberger/wingman/models/providers"
)

func main() {
	client := provider.NewClient(nil)
	model := models.ModelRef{Provider: "opencode", ID: "claude-sonnet-4-6"}

	request := models.Request{
		Model:      model,
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

	if response.Usage != nil {
		fmt.Printf("\nTokens: %d input, %d output, %d context\n", response.Usage.InputTokens, response.Usage.OutputTokens, response.Usage.ContextTokens())
		if info, ok := catalog.Get(model.Provider, model.ID); ok && info.ContextWindow > 0 {
			fmt.Printf("Context: %d / %d tokens (%d%% full)\n", response.Usage.ContextTokens(), info.ContextWindow, int(math.Round(response.Usage.ContextPercent(info.ContextWindow))))
		}
	}
}
