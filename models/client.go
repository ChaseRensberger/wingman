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
	return stream.Drain()
}

// ParseModelRef splits a provider-qualified model ref with an optional variant.
func ParseModelRef(ref string) (ModelRef, bool) {
	providerEnd := -1
	for i := 0; i < len(ref); i++ {
		if ref[i] == '#' {
			return ModelRef{}, false
		}
		if ref[i] == '/' {
			providerEnd = i
			break
		}
	}
	if providerEnd <= 0 || providerEnd+1 >= len(ref) {
		return ModelRef{}, false
	}
	provider := ref[:providerEnd]
	remainder := ref[providerEnd+1:]
	variantStart := -1
	for i := 0; i < len(remainder); i++ {
		if remainder[i] == '#' {
			if variantStart >= 0 {
				return ModelRef{}, false
			}
			variantStart = i
		}
	}
	if variantStart < 0 {
		return ModelRef{Provider: provider, ID: remainder}, true
	}
	if variantStart == 0 || variantStart+1 >= len(remainder) {
		return ModelRef{}, false
	}
	return ModelRef{Provider: provider, ID: remainder[:variantStart], Variant: remainder[variantStart+1:]}, true
}
