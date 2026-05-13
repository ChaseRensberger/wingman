package models

import "context"

// PreparedRequest is the provider-native request body and metadata produced
// without sending a network request.
type PreparedRequest struct {
	Model    ModelRef          `json:"model"`
	API      API               `json:"api"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers,omitempty"`
	Body     map[string]any    `json:"body"`
	Metadata map[string]any    `json:"metadata,omitempty"`
}

// Client is the new WingModels entry point. It exists alongside Model while
// agent/session callers migrate from provider-owned model instances to
// provider-qualified model refs.
type Client interface {
	Prepare(context.Context, Request) (*PreparedRequest, error)
	Stream(context.Context, Request) (*EventStream[StreamPart, *Message], error)
	Generate(context.Context, Request) (*Message, error)
}

// Generate drains Client.Stream and returns the final assistant message.
func Generate(ctx context.Context, c Client, req Request) (*Message, error) {
	stream, err := c.Stream(ctx, req)
	if err != nil {
		return nil, err
	}
	for range stream.Iter() {
	}
	return stream.Final()
}

// ParseModelRef splits a provider-qualified model ref like "openai/gpt-5.5".
func ParseModelRef(ref string) (ModelRef, bool) {
	for i := 0; i < len(ref); i++ {
		if ref[i] == '/' && i > 0 && i+1 < len(ref) {
			return ModelRef{Provider: ref[:i], ID: ref[i+1:]}, true
		}
	}
	return ModelRef{}, false
}
