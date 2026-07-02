package models

import (
	"encoding/json"
	"fmt"
	"sync"
)

// ------------------------------------------------------------------
// StreamPart
// ------------------------------------------------------------------

// StreamPart is the closed union of values emitted by Model.Stream.
type StreamPart interface {
	isStreamPart()
}

func (StreamStartPart) isStreamPart()      {}
func (TextStartPart) isStreamPart()        {}
func (TextDeltaPart) isStreamPart()        {}
func (TextEndPart) isStreamPart()          {}
func (ReasoningStartPart) isStreamPart()   {}
func (ReasoningDeltaPart) isStreamPart()   {}
func (ReasoningEndPart) isStreamPart()     {}
func (ToolInputStartPart) isStreamPart()   {}
func (ToolInputDeltaPart) isStreamPart()   {}
func (ToolInputEndPart) isStreamPart()     {}
func (ToolCallPart_) isStreamPart()        {}
func (FinishPart) isStreamPart()           {}
func (ErrorPart) isStreamPart()            {}
func (ResponseMetadataPart) isStreamPart() {}

// StreamStartPart signals the beginning of a stream.
type StreamStartPart struct{}

// TextStartPart begins a text block.
type TextStartPart struct {
	ID               string `json:"id"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
}

// TextDeltaPart carries an incremental text fragment.
type TextDeltaPart struct {
	ID               string `json:"id"`
	Delta            string `json:"delta"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
}

// TextEndPart ends a text block.
type TextEndPart struct {
	ID               string `json:"id"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
}

// ReasoningStartPart begins a reasoning block.
type ReasoningStartPart struct {
	ID               string `json:"id"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
}

// ReasoningDeltaPart carries an incremental reasoning fragment.
type ReasoningDeltaPart struct {
	ID               string `json:"id"`
	Delta            string `json:"delta"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
}

// ReasoningEndPart ends a reasoning block.
type ReasoningEndPart struct {
	ID               string `json:"id"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
}

// ToolInputStartPart begins a tool-call argument block.
type ToolInputStartPart struct {
	ID               string `json:"id"`
	ToolName         string `json:"tool_name"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
}

// ToolInputDeltaPart carries an incremental tool argument fragment.
type ToolInputDeltaPart struct {
	ID               string `json:"id"`
	Delta            string `json:"delta"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
}

// ToolInputEndPart ends a tool-call argument block.
type ToolInputEndPart struct {
	ID               string `json:"id"`
	ProviderMetadata Meta   `json:"provider_metadata,omitempty"`
}

// ToolCallPart_ is a stream event representing a completed tool call.
type ToolCallPart_ struct {
	ID               string         `json:"id"`
	ToolName         string         `json:"tool_name"`
	Input            map[string]any `json:"input"`
	ProviderExecuted bool           `json:"provider_executed,omitempty"`
	ProviderMetadata Meta           `json:"provider_metadata,omitempty"`
}

// FinishPart terminates the stream and carries the assembled message.
type FinishPart struct {
	Reason  FinishReason `json:"reason"`
	Usage   Usage        `json:"usage"`
	Message *Message     `json:"message,omitempty"`
}

// ErrorPart signals a terminal stream error.
type ErrorPart struct {
	Error string `json:"error"`
}

// ResponseMetadataPart carries provider-specific metadata mid-stream.
type ResponseMetadataPart struct {
	Meta map[string]any `json:"meta"`
}

// ------------------------------------------------------------------
// StreamPart registry
// ------------------------------------------------------------------

var (
	streamPartRegistryMu sync.RWMutex
	streamPartRegistry   = map[string]func(data []byte) (StreamPart, error){
		"stream_start": func(data []byte) (StreamPart, error) {
			var p StreamStartPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"text_start": func(data []byte) (StreamPart, error) {
			var p TextStartPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"text_delta": func(data []byte) (StreamPart, error) {
			var p TextDeltaPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"text_end": func(data []byte) (StreamPart, error) {
			var p TextEndPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"reasoning_start": func(data []byte) (StreamPart, error) {
			var p ReasoningStartPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"reasoning_delta": func(data []byte) (StreamPart, error) {
			var p ReasoningDeltaPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"reasoning_end": func(data []byte) (StreamPart, error) {
			var p ReasoningEndPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"tool_input_start": func(data []byte) (StreamPart, error) {
			var p ToolInputStartPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"tool_input_delta": func(data []byte) (StreamPart, error) {
			var p ToolInputDeltaPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"tool_input_end": func(data []byte) (StreamPart, error) {
			var p ToolInputEndPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"tool_call": func(data []byte) (StreamPart, error) {
			var p ToolCallPart_
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"finish": func(data []byte) (StreamPart, error) {
			var p FinishPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"error": func(data []byte) (StreamPart, error) {
			var p ErrorPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
		"response_metadata": func(data []byte) (StreamPart, error) {
			var p ResponseMetadataPart
			err := json.Unmarshal(data, &p)
			return p, err
		},
	}
)

func streamPartTypeName(p StreamPart) string {
	switch p.(type) {
	case StreamStartPart:
		return "stream_start"
	case TextStartPart:
		return "text_start"
	case TextDeltaPart:
		return "text_delta"
	case TextEndPart:
		return "text_end"
	case ReasoningStartPart:
		return "reasoning_start"
	case ReasoningDeltaPart:
		return "reasoning_delta"
	case ReasoningEndPart:
		return "reasoning_end"
	case ToolInputStartPart:
		return "tool_input_start"
	case ToolInputDeltaPart:
		return "tool_input_delta"
	case ToolInputEndPart:
		return "tool_input_end"
	case ToolCallPart_:
		return "tool_call"
	case FinishPart:
		return "finish"
	case ErrorPart:
		return "error"
	case ResponseMetadataPart:
		return "response_metadata"
	default:
		return ""
	}
}

func MarshalStreamPart(p StreamPart) ([]byte, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	m["type"] = streamPartTypeName(p)
	return json.Marshal(m)
}

func UnmarshalStreamPart(data []byte) (StreamPart, error) {
	var wrapper struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	streamPartRegistryMu.RLock()
	fn, ok := streamPartRegistry[wrapper.Type]
	streamPartRegistryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown stream part type: %s", wrapper.Type)
	}
	return fn(data)
}
