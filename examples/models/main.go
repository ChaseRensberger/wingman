package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/catalog"
	"github.com/chaserensberger/wingman/models/providers"

	_ "github.com/chaserensberger/wingman/models/providers/anthropic"
	_ "github.com/chaserensberger/wingman/models/providers/openai"
	_ "github.com/chaserensberger/wingman/models/providers/opencode"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Supported built-in catalog refs today:
	//   - openai/gpt-5.5
	//   - anthropic/claude-sonnet-4-6
	//   - opencode/claude-sonnet-4-6
	ref, ok := models.ParseModelRef("opencode/claude-sonnet-4-6")
	if !ok {
		log.Fatal("invalid model ref")
	}

	info, ok := catalog.Get(ref.Provider, ref.ID)
	if !ok {
		log.Fatalf("model not found in catalog: %s", ref.Ref())
	}
	ref.API = info.API
	ref.BaseURL = info.BaseURL

	client := provider.NewClient(map[string]string{
		// Optional. If omitted, the client falls back to the catalog auth_env:
		// OPENAI_API_KEY, ANTHROPIC_API_KEY, or OPENCODE_API_KEY.
		"opencode": os.Getenv("OPENCODE_API_KEY"),
	})

	req := models.Request{
		Model:  ref,
		System: "You are concise.",
		Messages: []models.Message{
			models.NewUserText("Say hello in one short sentence."),
		},
		Generation: models.Generation{MaxTokens: 80},
	}

	prepared, err := client.Prepare(ctx, req)
	if err != nil {
		log.Fatalf("prepare: %v", err)
	}
	pretty, _ := json.MarshalIndent(prepared, "", "  ")
	fmt.Printf("Prepared request:\n%s\n\n", pretty)

	if os.Getenv(info.AuthEnv) == "" {
		fmt.Printf("Set %s to run the live request.\n", info.AuthEnv)
		return
	}

	msg, err := client.Generate(ctx, req)
	if err != nil {
		log.Fatalf("generate: %v", err)
	}
	fmt.Println("Response:")
	for _, part := range msg.Content {
		if text, ok := part.(models.TextPart); ok {
			fmt.Println(text.Text)
		}
	}
}
