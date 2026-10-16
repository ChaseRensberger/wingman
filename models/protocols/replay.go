package protocols

import (
	"fmt"
	"strings"

	"github.com/chaserensberger/wingman/models"
)

func sameReplayModel(info models.ModelInfo, origin *models.MessageOrigin) bool {
	return origin != nil && info.Provider != "" && info.ID != "" && info.API != "" &&
		origin.Provider == info.Provider && origin.ModelID == info.ID && origin.API == info.API
}

// replayMessages derives request-only history; provider state belongs to its producing model.
func replayMessages(info models.ModelInfo, messages []models.Message) []models.Message {
	reserved, used := map[string]bool{}, map[string]bool{}
	for _, message := range messages {
		for _, call := range toolCalls(message.Content) {
			reserved[call.CallID] = true
		}
	}
	ids := map[string][]string{}
	nextID := 0
	out := make([]models.Message, 0, len(messages))
	for _, message := range messages {
		same := sameReplayModel(info, message.Origin)
		content := make(models.Content, 0, len(message.Content))
		for _, part := range message.Content {
			switch p := part.(type) {
			case models.ReasoningPart:
				if !same {
					continue
				}
			case models.TextPart:
				if !same {
					if p.Text == "" {
						continue
					}
					p.ProviderMetadata = nil
				}
				part = p
			case models.ImagePart:
				content = append(content, replayImages(models.Content{p}, info.Capabilities.Images)...)
				continue
			case models.ToolCallPart:
				key := replayCallKey(p.CallID, p.ToolUseID, p.ID)
				if !validReplayCallID(info, p.CallID) || used[p.CallID] {
					for {
						nextID++
						id := fmt.Sprintf("w%08d", nextID)
						if !reserved[id] && !used[id] {
							p.CallID = id
							break
						}
					}
				}
				used[p.CallID] = true
				ids[key] = append(ids[key], p.CallID)
				if !same {
					p.ProviderMetadata = nil
				}
				part = p
			case models.ToolResultPart:
				key := replayCallKey(p.CallID, p.ToolUseID, p.ID)
				if pending := ids[key]; len(pending) > 0 {
					p.CallID = pending[0]
					ids[key] = pending[1:]
				}
				p.Output = replayImages(p.Output, info.Capabilities.Images)
				if !same {
					p.ProviderMetadata = nil
				}
				part = p
			}
			content = append(content, part)
		}
		if message.Role == models.RoleAssistant && len(content) == 0 {
			continue
		}
		message.Content = content
		out = append(out, message)
	}
	return out
}

func replayCallKey(callID, toolUseID, partID string) string {
	if toolUseID != "" {
		return "use:" + toolUseID
	}
	if partID != "" {
		return "part:" + partID
	}
	return "call:" + callID
}

func validReplayCallID(info models.ModelInfo, id string) bool {
	limit := 64
	mistral := false
	if info.API == models.APIOpenAICompletions || info.API == models.APIOpenAICompatible {
		limit = 40
		for _, family := range []string{"mistral", "devstral", "codestral", "pixtral", "mixtral"} {
			mistral = mistral || strings.Contains(strings.ToLower(info.ID), family)
		}
	}
	if len(id) == 0 || len(id) > limit || (mistral && len(id) != 9) {
		return false
	}
	for _, c := range id {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || (!mistral && (c == '_' || c == '-')) {
			continue
		}
		return false
	}
	return true
}

func replayImages(content models.Content, vision bool) models.Content {
	out := make(models.Content, 0, len(content))
	for _, part := range content {
		if image, ok := part.(models.ImagePart); ok {
			if !vision {
				out = append(out, models.TextPart{Text: "(image omitted: model does not support images)"})
				continue
			}
			image.ProviderMetadata = nil
			if strings.HasPrefix(image.URL, "data:") {
				header, data, ok := strings.Cut(strings.TrimPrefix(image.URL, "data:"), ",")
				if ok && strings.HasSuffix(header, ";base64") {
					image.URL, image.Base64, image.MediaType = "", data, strings.TrimSuffix(header, ";base64")
				}
			}
			part = image
		}
		out = append(out, part)
	}
	return out
}

func splitToolImages(result models.ToolResultPart) (models.ToolResultPart, models.Content) {
	var images models.Content
	output := make(models.Content, 0, len(result.Output))
	for _, part := range result.Output {
		if _, ok := part.(models.ImagePart); ok {
			images = append(images, part)
			output = append(output, models.TextPart{Text: "(image attached after tool results)"})
		} else {
			output = append(output, part)
		}
	}
	if len(images) == 0 {
		return result, nil
	}
	result.Output = output
	caption := models.TextPart{Text: fmt.Sprintf("Images from tool call %s (%s):", result.CallID, result.Name)}
	return result, append(models.Content{caption}, images...)
}
