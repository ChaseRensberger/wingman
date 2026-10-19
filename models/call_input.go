package models

import (
	"encoding/json"
	"fmt"
)

// CallInput is model-facing content after context transforms and before provider translation.
type CallInput struct {
	Instructions []InstructionSnapshot `json:"instructions,omitempty"`
	System       string                `json:"system"`
	Messages     []Message             `json:"messages"`
	Tools        []ToolDef             `json:"tools"`
	ToolChoice   ToolChoice            `json:"tool_choice,omitempty"`
	OutputSchema *OutputSchema         `json:"output_schema,omitempty"`
}

type callInputManifest struct {
	Instructions []string        `json:"instructions,omitempty"`
	System       string          `json:"system"`
	Messages     []string        `json:"messages"`
	Tools        []string        `json:"tools"`
	ToolChoice   ToolChoice      `json:"tool_choice,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

// SnapshotCallInput freezes request content into a manifest and content-addressed blobs.
func SnapshotCallInput(req Request) (string, map[string]json.RawMessage, error) {
	blobs := make(map[string]json.RawMessage)
	put := func(value any) (string, error) {
		data, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		hash := sha256hex(data)
		blobs[hash] = data
		return hash, nil
	}
	manifest := callInputManifest{Messages: []string{}, Tools: []string{}, ToolChoice: req.ToolChoice}
	var err error
	manifest.System, err = put(req.System)
	if err != nil {
		return "", nil, err
	}
	for _, message := range req.Messages {
		hash, err := put(message)
		if err != nil {
			return "", nil, fmt.Errorf("snapshot input message: %w", err)
		}
		manifest.Messages = append(manifest.Messages, hash)
	}
	for _, tool := range req.Tools {
		hash, err := put(tool)
		if err != nil {
			return "", nil, fmt.Errorf("snapshot input tool: %w", err)
		}
		manifest.Tools = append(manifest.Tools, hash)
	}
	for _, source := range req.Instructions {
		hash, err := put(source)
		if err != nil {
			return "", nil, err
		}
		manifest.Instructions = append(manifest.Instructions, hash)
	}
	if req.OutputSchema != nil {
		manifest.OutputSchema, err = json.Marshal(req.OutputSchema)
		if err != nil {
			return "", nil, err
		}
	}
	hash, err := put(manifest)
	return hash, blobs, err
}

// ResolveCallInput reconstructs a frozen input using its content-addressed blobs.
func ResolveCallInput(hash string, read func(string) (json.RawMessage, error)) (*CallInput, error) {
	decode := func(hash string, target any) error {
		data, err := read(hash)
		if err != nil {
			return err
		}
		if sha256hex(data) != hash {
			return fmt.Errorf("model input blob %s is missing or corrupt", hash)
		}
		return json.Unmarshal(data, target)
	}
	var manifest callInputManifest
	if err := decode(hash, &manifest); err != nil {
		return nil, err
	}
	input := &CallInput{Messages: []Message{}, Tools: []ToolDef{}, ToolChoice: manifest.ToolChoice}
	for _, hash := range manifest.Instructions {
		var source InstructionSnapshot
		if err := decode(hash, &source); err != nil {
			return nil, err
		}
		input.Instructions = append(input.Instructions, source)
	}
	if err := decode(manifest.System, &input.System); err != nil {
		return nil, err
	}
	for _, hash := range manifest.Messages {
		var message Message
		if err := decode(hash, &message); err != nil {
			return nil, err
		}
		input.Messages = append(input.Messages, message)
	}
	for _, hash := range manifest.Tools {
		var tool ToolDef
		if err := decode(hash, &tool); err != nil {
			return nil, err
		}
		input.Tools = append(input.Tools, tool)
	}
	if len(manifest.OutputSchema) > 0 {
		if err := json.Unmarshal(manifest.OutputSchema, &input.OutputSchema); err != nil {
			return nil, err
		}
	}
	return input, nil
}

// InstructionSnapshot records guidance and the read that caused its discovery.
type InstructionSnapshot struct {
	Kind          string `json:"kind"`
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	Content       string `json:"content,omitempty"`
	RequestedPath string `json:"requested_path,omitempty"`
	ResolvedPath  string `json:"resolved_path,omitempty"`
	ToolUseID     string `json:"tool_use_id,omitempty"`
	CallID        string `json:"call_id,omitempty"`
}
