package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"github.com/chaserensberger/wingman/wingagent/session"
	"github.com/chaserensberger/wingman/wingmodels/providers/anthropic"
)

type Person struct {
	Name       string   `json:"name"`
	Age        int      `json:"age"`
	Occupation string   `json:"occupation"`
	Hobbies    []string `json:"hobbies"`
}

func main() {
	godotenv.Load(".env.local")

	p, err := anthropic.New(anthropic.Config{})
	if err != nil {
		log.Fatalf("failed to create Anthropic provider: %v", err)
	}

	// NOTE: structured-output enforcement (json_schema response_format)
	// is deferred to a later tier. For now we coerce by prompt.
	system := `Extract person information from the given text. Return ONLY a JSON object with fields:
{"name": string, "age": integer, "occupation": string, "hobbies": [string]}
No prose, no markdown fencing.`

	s := session.New(
		session.WithModel(p),
		session.WithSystem(system),
	)

	ctx := context.Background()
	result, err := s.Run(ctx, "John Smith is a 34 year old software engineer who enjoys hiking, photography, and playing chess in his spare time.")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Raw response:")
	fmt.Println(result.Response)

	var person Person
	if err := json.Unmarshal([]byte(result.Response), &person); err != nil {
		log.Printf("Failed to parse JSON: %v", err)
		return
	}

	fmt.Println("\nParsed person:")
	fmt.Printf("  Name: %s\n", person.Name)
	fmt.Printf("  Age: %d\n", person.Age)
	fmt.Printf("  Occupation: %s\n", person.Occupation)
	fmt.Printf("  Hobbies: %v\n", person.Hobbies)
}
