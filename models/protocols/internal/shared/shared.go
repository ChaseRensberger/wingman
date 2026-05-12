package shared

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/chaserensberger/wingman/models"
)

// Origin builds the normalized origin stamped on assistant messages.
func Origin(provider string, api models.API, modelID string) *models.MessageOrigin {
	return &models.MessageOrigin{Provider: provider, API: api, ModelID: modelID}
}

// CountTokens returns a char-based approximation (4 chars ~= 1 token).
func CountTokens(msgs []models.Message) int {
	total := 0
	for _, m := range msgs {
		for _, p := range m.Content {
			switch v := p.(type) {
			case models.TextPart:
				total += len(v.Text)
			case models.ReasoningPart:
				total += len(v.Reasoning)
			case models.ToolCallPart:
				total += len(v.Name) + 8
				if b, err := json.Marshal(v.Input); err == nil {
					total += len(b)
				}
			case models.ToolResultPart:
				total += len(ToolResultText(v.Output))
			}
		}
	}
	return total / 4
}

// ToolResultText flattens tool result parts into provider-friendly text.
func ToolResultText(out []models.Part) string {
	var sb strings.Builder
	for _, p := range out {
		if t, ok := p.(models.TextPart); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String()
}

// SplitCompositeID splits IDs stored as callID|providerItemID.
func SplitCompositeID(id string) (callID, itemID string) {
	if i := strings.IndexByte(id, '|'); i >= 0 {
		return id[:i], id[i+1:]
	}
	return id, ""
}

// ScanSSE scans Server-Sent Events and calls handle with event and data.
func ScanSSE(ctx context.Context, resp *http.Response, handle func(eventType, data string) bool, onError func(error), missingTerminator string) {
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventType string
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			onError(err)
			return
		}
		line := scanner.Text()
		switch {
		case line == "":
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" || handle(eventType, data) {
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		onError(err)
		return
	}
	onError(fmt.Errorf("%s", missingTerminator))
}

// CloseError emits a minimal error stream and closes it.
func CloseError(out *models.EventStream[models.StreamPart, *models.Message], origin *models.MessageOrigin, err error) {
	msg := &models.Message{Role: models.RoleAssistant, Origin: origin, FinishReason: models.FinishReasonError}
	out.Push(models.StreamStartPart{})
	out.Push(models.ErrorPart{Message: err.Error()})
	out.Push(models.FinishPart{Reason: models.FinishReasonError, Message: msg})
	out.Close(msg, err)
}
