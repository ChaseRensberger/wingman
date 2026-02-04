package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"wingman/models"
)

type Stream struct {
	resp         *http.Response
	scanner      *bufio.Scanner
	currentEvent models.StreamEvent
	err          error
	closed       bool

	accumulatedResponse *models.WingmanInferenceResponse
	contentBlocks       []models.WingmanContentBlock
	currentBlockIndex   int
	currentBlockText    strings.Builder
	currentBlockJSON    strings.Builder
	currentToolUse      *models.WingmanContentBlock
}

func newStream(resp *http.Response) *Stream {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	return &Stream{
		resp:    resp,
		scanner: scanner,
		accumulatedResponse: &models.WingmanInferenceResponse{
			Content: []models.WingmanContentBlock{},
		},
		contentBlocks: []models.WingmanContentBlock{},
	}
}

func (s *Stream) Next() bool {
	if s.err != nil || s.closed {
		return false
	}

	var eventType string

	for s.scanner.Scan() {
		line := s.scanner.Text()

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			event, done := s.parseEvent(eventType, data)
			if event != nil {
				s.currentEvent = *event
				return true
			}
			if done {
				return false
			}
		}
	}

	if err := s.scanner.Err(); err != nil {
		s.err = err
	}

	return false
}

func (s *Stream) parseEvent(eventType, data string) (*models.StreamEvent, bool) {
	switch eventType {
	case "message_start":
		var event struct {
			Message struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			s.err = fmt.Errorf("failed to parse message_start: %w", err)
			return nil, true
		}
		s.accumulatedResponse.ID = event.Message.ID
		s.accumulatedResponse.Usage.InputTokens = event.Message.Usage.InputTokens
		return &models.StreamEvent{Type: models.EventMessageStart}, false

	case "content_block_start":
		var event struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type  string         `json:"type"`
				ID    string         `json:"id,omitempty"`
				Name  string         `json:"name,omitempty"`
				Text  string         `json:"text,omitempty"`
				Input map[string]any `json:"input,omitempty"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			s.err = fmt.Errorf("failed to parse content_block_start: %w", err)
			return nil, true
		}

		s.currentBlockIndex = event.Index
		s.currentBlockText.Reset()
		s.currentBlockJSON.Reset()

		if event.ContentBlock.Type == "tool_use" {
			s.currentToolUse = &models.WingmanContentBlock{
				Type: models.ContentTypeToolUse,
				ID:   event.ContentBlock.ID,
				Name: event.ContentBlock.Name,
			}
		} else {
			s.currentToolUse = nil
		}

		return &models.StreamEvent{
			Type:  models.EventContentBlockStart,
			Index: event.Index,
			ContentBlock: &models.StreamContentBlock{
				Type: event.ContentBlock.Type,
				ID:   event.ContentBlock.ID,
				Name: event.ContentBlock.Name,
				Text: event.ContentBlock.Text,
			},
		}, false

	case "content_block_delta":
		var event struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text,omitempty"`
				PartialJSON string `json:"partial_json,omitempty"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			s.err = fmt.Errorf("failed to parse content_block_delta: %w", err)
			return nil, true
		}

		if event.Delta.Type == "text_delta" {
			s.currentBlockText.WriteString(event.Delta.Text)
			return &models.StreamEvent{
				Type:  models.EventTextDelta,
				Text:  event.Delta.Text,
				Index: event.Index,
			}, false
		} else if event.Delta.Type == "input_json_delta" {
			s.currentBlockJSON.WriteString(event.Delta.PartialJSON)
			return &models.StreamEvent{
				Type:      models.EventInputJSONDelta,
				InputJSON: event.Delta.PartialJSON,
				Index:     event.Index,
			}, false
		}
		return nil, false

	case "content_block_stop":
		var event struct {
			Index int `json:"index"`
		}
		json.Unmarshal([]byte(data), &event)

		if s.currentToolUse != nil {
			var input map[string]any
			if s.currentBlockJSON.Len() > 0 {
				json.Unmarshal([]byte(s.currentBlockJSON.String()), &input)
			}
			s.currentToolUse.Input = input
			s.contentBlocks = append(s.contentBlocks, *s.currentToolUse)
		} else if s.currentBlockText.Len() > 0 {
			s.contentBlocks = append(s.contentBlocks, models.WingmanContentBlock{
				Type: models.ContentTypeText,
				Text: s.currentBlockText.String(),
			})
		}

		return &models.StreamEvent{
			Type:  models.EventContentBlockStop,
			Index: event.Index,
		}, false

	case "message_delta":
		var event struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			s.err = fmt.Errorf("failed to parse message_delta: %w", err)
			return nil, true
		}

		s.accumulatedResponse.StopReason = event.Delta.StopReason
		s.accumulatedResponse.Usage.OutputTokens = event.Usage.OutputTokens

		return &models.StreamEvent{
			Type:       models.EventMessageDelta,
			StopReason: event.Delta.StopReason,
			Usage: &models.WingmanUsage{
				InputTokens:  s.accumulatedResponse.Usage.InputTokens,
				OutputTokens: event.Usage.OutputTokens,
			},
		}, false

	case "message_stop":
		s.accumulatedResponse.Content = s.contentBlocks
		return &models.StreamEvent{Type: models.EventMessageStop}, true

	case "ping":
		return &models.StreamEvent{Type: models.EventPing}, false

	case "error":
		var event struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			s.err = fmt.Errorf("stream error: %s", data)
		} else {
			s.err = fmt.Errorf("%s: %s", event.Error.Type, event.Error.Message)
		}
		return &models.StreamEvent{Type: models.EventError, Error: s.err}, true

	default:
		return nil, false
	}
}

func (s *Stream) Event() models.StreamEvent {
	return s.currentEvent
}

func (s *Stream) Err() error {
	return s.err
}

func (s *Stream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if s.resp != nil && s.resp.Body != nil {
		return s.resp.Body.Close()
	}
	return nil
}

func (s *Stream) Response() *models.WingmanInferenceResponse {
	return s.accumulatedResponse
}

var _ io.Closer = (*Stream)(nil)
