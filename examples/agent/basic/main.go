package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"

	"wingman/agent"
	"wingman/provider/anthropic"
	"wingman/tool"
	"wingman/utils"
)

func main() {
	godotenv.Load(".env.local")

	workDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	provider, err := anthropic.New(anthropic.Config{})
	if err != nil {
		log.Fatal(err)
	}

	a, err := agent.New("WingmanAgent", provider,
		agent.WithInstructions("You are a helpful coding assistant. When asked to write code, use the write tool to create files. Use the bash tool to run commands."),
		agent.WithMaxTokens(4096),
		agent.WithTools(
			tool.NewBashTool(workDir),
			tool.NewReadTool(workDir),
			tool.NewWriteTool(workDir),
			tool.NewEditTool(workDir),
			tool.NewGlobTool(workDir),
			tool.NewGrepTool(workDir),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	prompt := "Write a Python script called fibonacci.py that calculates fibonacci numbers up to n (passed as command line argument), then run it with n=10"

	utils.UserPrint(prompt)
	fmt.Println()

	result, err := a.Run(ctx, prompt)
	if err != nil {
		log.Fatal(err)
	}

	for _, tc := range result.ToolCalls {
		if tc.Error != nil {
			utils.ToolPrint(fmt.Sprintf("[%s] Error: %v", tc.ToolName, tc.Error))
		} else {
			utils.ToolPrint(fmt.Sprintf("[%s] %s", tc.ToolName, truncate(tc.Output, 200)))
		}
	}

	fmt.Println()
	utils.AgentPrint(result.Response)
	fmt.Println()
	utils.ToolPrint(fmt.Sprintf("Steps: %d | Tokens - Input: %d, Output: %d",
		result.Steps, result.Usage.InputTokens, result.Usage.OutputTokens))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
