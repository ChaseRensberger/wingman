package ui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type ToolDisplay struct {
	Name     string
	Input    map[string]any
	Output   string
	Status   string
	Error    string
	Duration string
}

func RenderTool(tool ToolDisplay, styles Styles, width int) string {
	var sb strings.Builder

	icon := getToolIcon(tool.Name)
	status := getStatusIndicator(tool.Status, styles)

	title := fmt.Sprintf("%s %s", icon, tool.Name)
	if subtitle := getToolSubtitle(tool.Name, tool.Input); subtitle != "" {
		title += fmt.Sprintf(": %s", subtitle)
	}

	sb.WriteString(styles.Tool.Render(title))
	sb.WriteString("\n")

	if tool.Error != "" {
		sb.WriteString(styles.ToolBox.Render(
			styles.Error.Render("✗ Error: ") + tool.Error,
		))
		sb.WriteString("\n")
		return sb.String()
	}

	var content strings.Builder

	if len(tool.Input) > 0 {
		content.WriteString(styles.ToolSection.Render("┌─ Input") + "\n")
		for key, value := range tool.Input {
			if key == "command" || key == "description" {
				content.WriteString(fmt.Sprintf("│  %s: %v\n", key, value))
			}
		}
	}

	if tool.Output != "" {
		if content.Len() > 0 {
			content.WriteString(styles.ToolSection.Render("├─ Output") + "\n")
		} else {
			content.WriteString(styles.ToolSection.Render("┌─ Output") + "\n")
		}

		outputLines := strings.Split(strings.TrimRight(tool.Output, "\n"), "\n")
		maxLines := 20
		if len(outputLines) > maxLines {
			for _, line := range outputLines[:maxLines] {
				content.WriteString("│  " + line + "\n")
			}
			content.WriteString(styles.Subtle.Render(fmt.Sprintf("│  ... (%d more lines)\n", len(outputLines)-maxLines)))
		} else {
			for _, line := range outputLines {
				content.WriteString("│  " + line + "\n")
			}
		}
	}

	if content.Len() > 0 {
		statusLine := fmt.Sprintf("└─ Status: %s", status)
		if tool.Duration != "" {
			statusLine += fmt.Sprintf(" (%s)", tool.Duration)
		}
		content.WriteString(styles.ToolSection.Render(statusLine) + "\n")

		sb.WriteString(styles.ToolBox.Render(content.String()))
	} else {
		sb.WriteString(styles.ToolBox.Render(status))
	}

	sb.WriteString("\n")
	return sb.String()
}

func getToolIcon(name string) string {
	icons := map[string]string{
		"bash":     "🔧",
		"read":     "👓",
		"write":    "✏️",
		"edit":     "📝",
		"glob":     "🔍",
		"grep":     "🔎",
		"task":     "🤖",
		"webfetch": "🌐",
	}
	if icon, ok := icons[name]; ok {
		return icon
	}
	return "🔧"
}

func getStatusIndicator(status string, styles Styles) string {
	switch status {
	case "running":
		return styles.Running.Render("⏳ Running")
	case "completed", "success":
		return styles.Success.Render("✓ Success")
	case "error", "failed":
		return styles.Error.Render("✗ Failed")
	default:
		return styles.Subtle.Render("◌ " + status)
	}
}

func getToolSubtitle(name string, input map[string]any) string {
	switch name {
	case "bash":
		if desc, ok := input["description"].(string); ok && desc != "" {
			return desc
		}
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 50 {
				return cmd[:47] + "..."
			}
			return cmd
		}
	case "read":
		if path, ok := input["filePath"].(string); ok {
			return filepath.Base(path)
		}
	case "write", "edit":
		if path, ok := input["filePath"].(string); ok {
			return filepath.Base(path)
		}
	case "glob":
		if pattern, ok := input["pattern"].(string); ok {
			return pattern
		}
	case "grep":
		if pattern, ok := input["pattern"].(string); ok {
			return pattern
		}
	case "task":
		if desc, ok := input["description"].(string); ok {
			return desc
		}
	case "webfetch":
		if url, ok := input["url"].(string); ok {
			if len(url) > 50 {
				return url[:47] + "..."
			}
			return url
		}
	}
	return ""
}

func FormatJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
