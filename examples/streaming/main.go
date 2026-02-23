package main

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"github.com/chaserensberger/wingman/agent"
	"github.com/chaserensberger/wingman/core"
	"github.com/chaserensberger/wingman/provider/anthropic"
	"github.com/chaserensberger/wingman/session"
)

func main() {
	godotenv.Load(".env.local")

	p, err := anthropic.New()
	if err != nil {
		log.Fatalf("failed to create Anthropic provider: %v", err)
	}

	a := agent.New("Storyteller",
		agent.WithInstructions("You are a creative storyteller. Write engaging, vivid stories."),
		agent.WithProvider(p),
	)

	s := session.New(
		session.WithAgent(a),
	)

	fmt.Println("Streaming story...")
	fmt.Println()

	stream, err := s.RunStream(context.Background(), "Write a very short story about a robot learning to paint.")
	if err != nil {
		log.Fatal(err)
	}

	for stream.Next() {
		event := stream.Event()
		switch event.Type {
		case core.EventTextDelta:
			fmt.Print(event.Text)
		case core.EventMessageStop:
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
