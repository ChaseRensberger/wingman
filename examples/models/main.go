package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
	"github.com/chaserensberger/wingman/models/providers"
	_ "github.com/chaserensberger/wingman/models/providers/anthropic"
)

func main() {
	client := provider.NewClient(nil)
	// automatic model resolution
	model := models.ModelRef{Provider: "opencode", ID: "claude-sonnet-4-6"}
	run(context.Background(), client, model)

	anthropicClient := provider.NewClient(map[string]string{
		"anthropic": os.Getenv("ANTHROPIC_API_KEY"),
	})
	// direct provider model resolution
	model = models.ModelRef{Provider: "anthropic", ID: "claude-sonnet-4-6"}
	run(context.Background(), anthropicClient, model)
}

func run(ctx context.Context, client *provider.Client, model models.ModelRef) {
	request := models.Request{
		Model:      model,
		System:     "You are concise.",
		Messages:   []models.Message{models.NewUserText("Say hello in one short sentence.")},
		Generation: models.Generation{MaxTokens: 40},
	}

	response, err := client.Generate(ctx, request)
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
