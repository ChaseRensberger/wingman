package httpmodel

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/chaserensberger/wingman/models"
)

func (m *Model) readSSE(ctx context.Context, r io.Reader, stream *models.EventStream[models.StreamPart, *models.Message]) (*models.Message, models.Usage, models.FinishReason, error) {
	state := parseState{provider: m.Info_.Provider, api: m.Info_.API, model: m.Info_.ID, finish: models.FinishReasonStop}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var data strings.Builder
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return state.message(), state.usage, state.finish, transportError(m.Info_.Provider, err)
		}
		line := scanner.Text()
		if line == "" {
			if err := m.handleSSEData(data.String(), &state, stream); err != nil {
				return state.message(), state.usage, state.finish, err
			}
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if data.Len() > 0 {
		if err := m.handleSSEData(data.String(), &state, stream); err != nil {
			return state.message(), state.usage, state.finish, err
		}
	}
	if err := scanner.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return state.message(), state.usage, state.finish, transportError(m.Info_.Provider, ctxErr)
		}
		if isTransportError(err) {
			return state.message(), state.usage, state.finish, transportError(m.Info_.Provider, err)
		}
		return state.message(), state.usage, state.finish, decodingError(m.Info_.Provider, "invalid SSE framing", err)
	}
	if len(state.toolBuf) > 0 {
		return state.message(), state.usage, state.finish, decodingError(m.Info_.Provider, "incomplete tool call at end of stream", nil)
	}
	closeOpenParts(&state, stream)
	return state.message(), state.usage, state.finish, nil
}

func (m *Model) handleSSEData(data string, state *parseState, stream *models.EventStream[models.StreamPart, *models.Message]) error {
	if data == "" || data == "[DONE]" {
		return nil
	}
	switch m.Protocol {
	case OpenAIChat:
		var event openAIChatEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return decodingError(m.Info_.Provider, "invalid OpenAI Chat SSE event", err)
		}
		return parseOpenAIChat(event, state, stream)
	}
	switch m.Protocol {
	case OpenAIResponses:
		var event openAIResponsesEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return decodingError(m.Info_.Provider, "invalid OpenAI Responses SSE event", err)
		}
		return parseOpenAIResponses(event, state, stream)
	case AnthropicMessages:
		var event anthropicEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return decodingError(m.Info_.Provider, "invalid Anthropic SSE event", err)
		}
		return parseAnthropic(event, state, stream)
	case GeminiGenerate:
		var event geminiEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return decodingError(m.Info_.Provider, "invalid Gemini SSE event", err)
		}
		return parseGemini(event, state, stream)
	}
	return nil
}
