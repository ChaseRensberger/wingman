package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"

	"wingman/agent"
	"wingman/provider/anthropic"
	"wingman/session"
	"wingman/tool"
)

func main() {
	godotenv.Load(".env.local")

	workDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	p := anthropic.New(anthropic.Config{})
	if p == nil {
		log.Fatal("ANTHROPIC_API_KEY not set")
	}

	a := agent.New("WingmanAgent",
		agent.WithInstructions("You are a helpful coding assistant. When asked to write code, use the write tool to create files. Use the bash tool to run commands."),
		agent.WithProvider(p),
		agent.WithTools(
			tool.NewBashTool(),
			tool.NewReadTool(),
			tool.NewWriteTool(),
			tool.NewEditTool(),
			tool.NewGlobTool(),
			tool.NewGrepTool(),
		),
	)

	s := session.New(
		session.WithWorkDir(workDir),
		session.WithAgent(a),
	)

	ctx := context.Background()
	message := "Write a Python script called fibonacci.py that calculates fibonacci numbers up to n (passed as command line argument), then run it with n=10"

	fmt.Printf("User: %s\n\n", message)

	result, err := s.Run(ctx, message)
	if err != nil {
		log.Fatal(err)
	}

	for _, tc := range result.ToolCalls {
		if tc.Error != nil {
			fmt.Printf("Tool: [%s] Error: %v\n", tc.ToolName, tc.Error)
		} else {
			fmt.Printf("Tool: [%s] %s\n", tc.ToolName, truncate(tc.Output, 200))
		}
	}

	fmt.Println()
	fmt.Printf("Agent: %s\n", result.Response)
	fmt.Println()
	fmt.Printf("Steps: %d | Tokens - Input: %d, Output: %d\n",
		result.Steps, result.Usage.InputTokens, result.Usage.OutputTokens)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
