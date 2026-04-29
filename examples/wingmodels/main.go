package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/chaserensberger/wingman/wingmodels"
	"github.com/chaserensberger/wingman/wingmodels/providers/anthropic"
	// Swap the import above for openai:
	// "github.com/chaserensberger/wingman/wingmodels/providers/openai"
)

func main() {
	ctx := context.Background()

	// Create a provider. Anthropic reads ANTHROPIC_API_KEY from the env.
	// For OpenAI, use openai.New() and set OPENAI_API_KEY.
	model, err := anthropic.New()
	if err != nil {
		log.Fatalf("create model: %v", err)
	}

	info := model.Info()
	fmt.Printf("Provider: %s | Model: %s | API: %s\n\n", info.Provider, info.ID, info.API)

	// -----------------------------------------------------------------
	// 1. Simple synchronous completion
	// -----------------------------------------------------------------
	fmt.Println("=== Sync completion ===")
	syncDemo(ctx, model)

	// -----------------------------------------------------------------
	// 2. Streaming
	// -----------------------------------------------------------------
	fmt.Println("\n=== Streaming ===")
	streamDemo(ctx, model)

	// -----------------------------------------------------------------
	// 3. Tools
	// -----------------------------------------------------------------
	fmt.Println("\n=== Tools ===")
	toolsDemo(ctx, model)

	// -----------------------------------------------------------------
	// 4. Structured output (JSON schema)
	// -----------------------------------------------------------------
	fmt.Println("\n=== Structured output ===")
	structuredDemo(ctx, model)
}

// syncDemo shows the simplest way to get a complete response.
func syncDemo(ctx context.Context, model wingmodels.Model) {
	req := wingmodels.Request{
		System: "You are a terse assistant.",
		Messages: []wingmodels.Message{
			wingmodels.NewUserText("What is Go?"),
		},
	}

	msg, err := wingmodels.Run(ctx, model, req)
	if err != nil {
		log.Fatalf("sync run: %v", err)
	}

	for _, part := range msg.Content {
		if t, ok := part.(wingmodels.TextPart); ok {
			fmt.Println(t.Text)
		}
	}
	fmt.Printf("(finish reason: %s)\n", msg.FinishReason)
}

// streamDemo consumes the raw event stream and prints text deltas as they arrive.
func streamDemo(ctx context.Context, model wingmodels.Model) {
	req := wingmodels.Request{
		System: "You are a terse assistant.",
		Messages: []wingmodels.Message{
			wingmodels.NewUserText("Count from 1 to 5 slowly."),
		},
	}

	stream, err := model.Stream(ctx, req)
	if err != nil {
		log.Fatalf("stream setup: %v", err)
	}

	// Iterate over every event in the stream.
	for part := range stream.Iter() {
		switch p := part.(type) {
		case wingmodels.TextDeltaPart:
			fmt.Print(p.Delta)
		case wingmodels.FinishPart:
			fmt.Printf("\n(finish reason: %s | tokens: %d in / %d out)\n",
				p.Reason, p.Usage.InputTokens, p.Usage.OutputTokens)
		}
	}

	// stream.Final() returns the assembled message and any terminal error.
	if _, err := stream.Final(); err != nil {
		log.Fatalf("stream error: %v", err)
	}
}

// toolsDemo advertises a calculator tool to the model and handles the call.
func toolsDemo(ctx context.Context, model wingmodels.Model) {
	req := wingmodels.Request{
		System: "You have access to a calculator. Use it for math.",
		Messages: []wingmodels.Message{
			wingmodels.NewUserText("What is 123 * 456?"),
		},
		Tools: []wingmodels.ToolDef{
			{
				Name:        "calculate",
				Description: "Evaluate a mathematical expression",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"expression": map[string]any{
							"type":        "string",
							"description": "The expression to evaluate, e.g. '123 * 456'",
						},
					},
					"required": []string{"expression"},
				},
			},
		},
	}

	msg, err := wingmodels.Run(ctx, model, req)
	if err != nil {
		log.Fatalf("tools run: %v", err)
	}

	// Print any tool calls the model made.
	for _, part := range msg.Content {
		switch p := part.(type) {
		case wingmodels.ToolCallPart:
			fmt.Printf("Tool call: %s(%s)\n", p.Name, mustJSON(p.Input))
			if p.Name == "calculate" {
				expr, _ := p.Input["expression"].(string)
				fmt.Printf("  Result: ask a real calculator about '%s'\n", expr)
			}
		case wingmodels.TextPart:
			fmt.Println(p.Text)
		}
	}
}

// structuredDemo constrains the model to emit JSON matching a schema.
func structuredDemo(ctx context.Context, model wingmodels.Model) {
	schema := &wingmodels.OutputSchema{
		Name: "planet",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":       map[string]any{"type": "string"},
				"position":   map[string]any{"type": "integer"},
				"moons":      map[string]any{"type": "integer"},
				"has_rings":  map[string]any{"type": "boolean"},
			},
			"required": []string{"name", "position", "moons", "has_rings"},
		},
	}

	req := wingmodels.Request{
		System: "Extract planet facts. Return ONLY valid JSON.",
		Messages: []wingmodels.Message{
			wingmodels.NewUserText("Tell me about Saturn."),
		},
		OutputSchema: schema,
	}

	msg, err := wingmodels.Run(ctx, model, req)
	if err != nil {
		log.Fatalf("structured run: %v", err)
	}

	for _, part := range msg.Content {
		if t, ok := part.(wingmodels.TextPart); ok {
			var planet map[string]any
			if err := json.Unmarshal([]byte(t.Text), &planet); err != nil {
				log.Printf("parse error: %v", err)
				fmt.Println("Raw:", t.Text)
				continue
			}
			pretty, _ := json.MarshalIndent(planet, "", "  ")
			fmt.Println(string(pretty))
		}
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
