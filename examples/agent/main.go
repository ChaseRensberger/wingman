package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"

	"wingman/internal/utils"
	"wingman/agent"
	"wingman/provider/claude"
	"wingman/session"
	"wingman/tool"
)

func main() {
	godotenv.Load(".env.local")

	workDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	p := claude.New(claude.Config{})
	if p == nil {
		log.Fatal("ANTHROPIC_API_KEY not set")
	}

	a := agent.New("WingmanAgent",
		agent.WithInstructions("You are a helpful coding assistant. When asked to write code, use the write tool to create files. Use the bash tool to run commands."),
		agent.WithMaxTokens(4096),
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
		session.WithProvider(p),
	)

	ctx := context.Background()
	prompt := "Write a Python script called fibonacci.py that calculates fibonacci numbers up to n (passed as command line argument), then run it with n=10"

	utils.UserPrint(prompt)
	fmt.Println()

	result, err := s.Run(ctx, prompt)
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
