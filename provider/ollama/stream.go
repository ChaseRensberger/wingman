package ollama

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/chaserensberger/wingman/core"
)

type Stream struct {
	resp         *http.Response
	scanner      *bufio.Scanner
	currentEvent core.StreamEvent
	err          error
	closed       bool

	accumulatedResponse *core.InferenceResponse
	contentText         strings.Builder
	toolCalls           []core.ContentBlock
	started             bool
	toolCallIndex       int
}

func newStream(resp *http.Response) *Stream {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	return &Stream{
		resp:    resp,
		scanner: scanner,
		accumulatedResponse: &core.InferenceResponse{
			Content: []core.ContentBlock{},
		},
	}
}

type streamChunk struct {
	Model              string      `json:"model"`
	CreatedAt          string      `json:"created_at"`
	Message            chatMessage `json:"message"`
	Done               bool        `json:"done"`
	DoneReason         string      `json:"done_reason"`
	PromptEvalCount    int         `json:"prompt_eval_count"`
	PromptEvalDuration int64       `json:"prompt_eval_duration"`
	EvalCount          int         `json:"eval_count"`
	EvalDuration       int64       `json:"eval_duration"`
}

func (s *Stream) Next() bool {
	if s.err != nil || s.closed {
		return false
	}

	for s.scanner.Scan() {
		line := s.scanner.Text()
		if line == "" {
			continue
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			s.err = fmt.Errorf("ollama: failed to parse stream chunk: %w", err)
			return false
		}

		event := s.parseChunk(chunk)
		if event != nil {
			s.currentEvent = *event
			return true
		}

		if chunk.Done {
			return false
		}
	}

	if err := s.scanner.Err(); err != nil {
		s.err = err
	}

	return false
}

func (s *Stream) parseChunk(chunk streamChunk) *core.StreamEvent {
	if !s.started {
		s.started = true
		s.accumulatedResponse.ID = chunk.CreatedAt
		return &core.StreamEvent{Type: core.EventMessageStart}
	}

	if len(chunk.Message.ToolCalls) > 0 {
		for _, tc := range chunk.Message.ToolCalls {
			toolBlock := core.ContentBlock{
				Type:  core.ContentTypeToolUse,
				ID:    fmt.Sprintf("tool_%d", s.toolCallIndex),
				Name:  tc.Function.Name,
				Input: tc.Function.Arguments,
			}
			s.toolCalls = append(s.toolCalls, toolBlock)

			event := &core.StreamEvent{
				Type:  core.EventContentBlockStart,
				Index: s.toolCallIndex,
				ContentBlock: &core.StreamContentBlock{
					Type: "tool_use",
					ID:   toolBlock.ID,
					Name: toolBlock.Name,
				},
			}
			s.toolCallIndex++
			return event
		}
	}

	if chunk.Message.Content != "" {
		s.contentText.WriteString(chunk.Message.Content)
		return &core.StreamEvent{
			Type:  core.EventTextDelta,
			Text:  chunk.Message.Content,
			Index: 0,
		}
	}

	if chunk.Done {
		var content []core.ContentBlock
		if s.contentText.Len() > 0 {
			content = append(content, core.ContentBlock{
				Type: core.ContentTypeText,
				Text: s.contentText.String(),
			})
		}
		content = append(content, s.toolCalls...)

		stopReason := chunk.DoneReason
		if len(s.toolCalls) > 0 {
			stopReason = "tool_use"
		}

		s.accumulatedResponse.Content = content
		s.accumulatedResponse.StopReason = stopReason
		s.accumulatedResponse.Usage = core.Usage{
			InputTokens:  chunk.PromptEvalCount,
			OutputTokens: chunk.EvalCount,
		}

		return &core.StreamEvent{
			Type:       core.EventMessageDelta,
			StopReason: stopReason,
			Usage: &core.Usage{
				InputTokens:  chunk.PromptEvalCount,
				OutputTokens: chunk.EvalCount,
			},
		}
	}

	return nil
}

func (s *Stream) Event() core.StreamEvent {
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

func (s *Stream) Response() *core.InferenceResponse {
	return s.accumulatedResponse
}

var _ io.Closer = (*Stream)(nil)
