package main

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"github.com/chaserensberger/wingman/wingagent/loop"
	"github.com/chaserensberger/wingman/wingagent/session"
	"github.com/chaserensberger/wingman/wingmodels"
	"github.com/chaserensberger/wingman/wingmodels/providers/anthropic"
)

func main() {
	godotenv.Load(".env.local")

	p, err := anthropic.New()
	if err != nil {
		log.Fatalf("failed to create Anthropic provider: %v", err)
	}

	s := session.New(
		session.WithModel(p),
		session.WithSystem("You are a creative storyteller. Write engaging, vivid stories."),
	)

	fmt.Println("Streaming story...")
	fmt.Println()

	stream, err := s.RunStream(context.Background(), "Write a very short story about a robot learning to paint.")
	if err != nil {
		log.Fatal(err)
	}

	for stream.Next() {
		event := stream.Event()
		if event.Type != "stream_part" {
			continue
		}
		spe, ok := event.Data.(loop.StreamPartEvent)
		if !ok {
			continue
		}
		switch part := spe.Part.(type) {
		case wingmodels.TextDeltaPart:
			fmt.Print(part.Delta)
		case wingmodels.FinishPart:
			fmt.Println()
		}
	}

	if err := stream.Err(); err != nil {
		log.Fatal(err)
	}

	result := stream.Result()
	fmt.Println()
	fmt.Printf("Steps: %d | Tokens - Input: %d, Output: %d\n",
		result.Steps, result.Usage.InputTokens, result.Usage.OutputTokens)
}
