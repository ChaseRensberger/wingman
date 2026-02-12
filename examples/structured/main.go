package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"wingman/agent"
	"wingman/provider/anthropic"
	"wingman/session"
)

type Person struct {
	Name       string   `json:"name"`
	Age        int      `json:"age"`
	Occupation string   `json:"occupation"`
	Hobbies    []string `json:"hobbies"`
}

func main() {
	godotenv.Load(".env.local")

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The person's full name",
			},
			"age": map[string]any{
				"type":        "integer",
				"description": "The person's age in years",
			},
			"occupation": map[string]any{
				"type":        "string",
				"description": "The person's job or profession",
			},
			"hobbies": map[string]any{
				"type":        "array",
				"description": "List of the person's hobbies",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
		"required":             []string{"name", "age", "occupation", "hobbies"},
		"additionalProperties": false,
	}

	a := agent.New("Extractor",
		agent.WithInstructions("Extract person information from the given text. Return only valid JSON."),
		agent.WithProvider(anthropic.New(anthropic.Config{})),
		agent.WithOutputSchema(schema),
	)

	s := session.New(
		session.WithAgent(a),
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
