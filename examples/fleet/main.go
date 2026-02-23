package main

import (
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"github.com/chaserensberger/wingman/actor"
	"github.com/chaserensberger/wingman/agent"
	"github.com/chaserensberger/wingman/provider/anthropic"
)

func main() {
	godotenv.Load(".env.local")

	p, err := anthropic.New(anthropic.Config{})
	if err != nil {
		log.Fatalf("failed to create Anthropic provider: %v", err)
	}

	a := agent.New("Calculator",
		agent.WithInstructions("You are a calculator. When given a math problem, solve it and respond with ONLY the numeric answer, nothing else."),
		agent.WithProvider(p),
	)

	fleet := actor.NewFleet(actor.FleetConfig{
		WorkerCount: 3,
		Agent:       a,
	})
	defer fleet.Shutdown()

	problems := []string{
		"What is 15 * 7?",
		"What is 123 + 456?",
		"What is 1000 / 8?",
		"What is 99 - 33?",
		"What is 12 * 12?",
	}

	fmt.Printf("Submitting %d problems to %d workers...\n\n", len(problems), 3)

	if err := fleet.SubmitAll(problems); err != nil {
		log.Fatal(err)
	}

	results := fleet.AwaitAll()

	fmt.Println("Results:")
	for i, r := range results {
		if r.Error != nil {
			fmt.Printf("  Problem %d: ERROR - %v\n", i, r.Error)
		} else {
			fmt.Printf("  Problem %d: %s (worker: %s, tokens: %d)\n",
				i, r.Result.Response, r.WorkerName, r.Result.Usage.OutputTokens)
		}
	}
}
