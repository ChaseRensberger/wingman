package ui

import (
	"fmt"
	"strings"
)

type Message struct {
	Role    string
	Content string
	Tools   []ToolDisplay
}

func RenderUserMessage(text string, styles Styles, width int) string {
	return styles.User.Render(fmt.Sprintf("> %s", text))
}

func RenderAssistantMessage(text string, styles Styles, width int) string {
	if text == "" {
		return ""
	}

	rendered := RenderMarkdown(text, styles, width)
	return styles.Assistant.Render(rendered)
}

func RenderMessage(msg Message, styles Styles, width int) string {
	var sb strings.Builder

	switch msg.Role {
	case "user":
		sb.WriteString(RenderUserMessage(msg.Content, styles, width))
		sb.WriteString("\n")

	case "assistant":
		if msg.Content != "" {
			sb.WriteString(RenderAssistantMessage(msg.Content, styles, width))
			sb.WriteString("\n")
		}

		for _, tool := range msg.Tools {
			sb.WriteString(RenderTool(tool, styles, width))
		}
	}

	return sb.String()
}

func RenderError(err error, styles Styles) string {
	return styles.Error.Render(fmt.Sprintf("✗ Error: %v", err))
}

func RenderHeader(text string, styles Styles) string {
	border := strings.Repeat("─", len(text)+4)
	return styles.Header.Render(fmt.Sprintf("╭%s╮\n│ %s │\n╰%s╯", border, text, border))
}

func RenderStatusBar(inputTokens, outputTokens int, sessionID string, styles Styles, width int) string {
	left := fmt.Sprintf("Tokens: %d in / %d out", inputTokens, outputTokens)
	right := "Ctrl+C to exit"

	spacing := width - len(left) - len(right) - 4
	if spacing < 1 {
		spacing = 1
	}

	content := left + strings.Repeat(" ", spacing) + right
	return styles.StatusBar.Render(content)
}
