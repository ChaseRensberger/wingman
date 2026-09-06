package route

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/chaserensberger/wingman/models"
)

// Prepare converts a common request without dispatching it or resolving secrets.
func (m *Model) Prepare(ctx context.Context, req models.Request) (*models.PreparedRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	req.Messages = models.ExpandToolMessages(models.NormalizeMessages(req.Messages))
	body, err := m.Route.Protocol.Body(m.Info_, req)
	if err != nil {
		return nil, err
	}
	body = Overlay(body, m.Variant.ProviderOptions)
	body = Overlay(body, m.Variant.HTTP.Body)
	body = Overlay(body, req.ProviderOptions[m.Info_.Provider])
	body = Overlay(body, req.HTTP.Body)
	if m.Route.Body != nil {
		body = m.Route.Body(body)
	}
	headers := mergeHeaders(map[string]string{"content-type": "application/json"}, m.Variant.HTTP.Headers, req.HTTP.Headers, m.Route.Headers)
	return &models.PreparedRequest{
		Model: models.ModelRef{Provider: m.Info_.Provider, ID: m.Info_.ID, Variant: m.Variant.ID, API: m.Info_.API, BaseURL: m.Route.Endpoint.BaseURL, Env: m.Info_.Env, ContextWindow: m.Info_.ContextWindow, MaxOutput: m.Info_.MaxOutput, Capabilities: m.Info_.Capabilities},
		API:   m.Info_.API, URL: m.Route.URL(MergeStrings(m.Variant.HTTP.Query, req.HTTP.Query)), Headers: headers, Body: body,
		Metadata: map[string]any{"route": m.Route.ID, "protocol": m.Route.Protocol.ID()},
	}, nil
}

// Stream dispatches a request and validates the normalized protocol lifecycle.
func (m *Model) Stream(ctx context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	prepared, err := m.Prepare(ctx, req)
	if err != nil {
		return nil, err
	}
	resp, err := m.Route.Transport.Open(ctx, m.Route, prepared)
	if err != nil {
		return nil, err
	}
	stream := models.NewEventStream[models.StreamPart, *models.Message](64)
	stream.BindContext(ctx)
	go func() {
		defer resp.Body.Close()
		stream.Push(models.StreamStartPart{})
		requestID := responseRequestID(resp.Header)
		if requestID != "" {
			stream.Push(models.ResponseMetadataPart{Meta: map[string]any{"request_id": requestID}})
		}
		parser := m.Route.Protocol.NewParser(m.Info_)
		terminal := false
		var finish *models.FinishPart
		emit := func(parts []models.StreamPart) error {
			for _, part := range parts {
				if terminal {
					return decodingError(m.Info_.Provider, "provider emitted data after completion", nil)
				}
				switch p := part.(type) {
				case models.FinishPart:
					terminal, finish = true, &p
				case models.ErrorPart:
					return &models.ProviderError{Provider: m.Info_.Provider, Category: models.ErrorProvider, Message: p.Error}
				default:
					stream.Push(part)
				}
			}
			return nil
		}
		var failure error
		for frame, err := range m.Route.Framing(ctx, resp.Body) {
			if err != nil {
				failure = transportError(m.Info_.Provider, err)
				break
			}
			parts, err := parser.Step(frame)
			if failure = emit(parts); failure != nil {
				break
			}
			if err != nil {
				failure = err
				break
			}
			if terminal {
				break
			}
		}
		if failure == nil && !terminal {
			parts, err := parser.End()
			failure = emit(parts)
			if failure == nil {
				failure = err
			}
		}
		if failure == nil && ctx.Err() != nil {
			failure = transportError(m.Info_.Provider, ctx.Err())
		}
		if failure == nil && !terminal {
			failure = decodingError(m.Info_.Provider, "provider response ended without a completion event", nil)
		}
		msg := parser.Message()
		if failure != nil {
			msg.FinishReason = ""
			var providerErr *models.ProviderError
			if errors.As(failure, &providerErr) {
				providerErr.RequestID, providerErr.Status = requestID, resp.StatusCode
			}
			stream.Push(models.ErrorPart{Error: failure.Error()})
			stream.Close(msg, failure)
			return
		}
		stream.Push(*finish)
		stream.Close(msg, nil)
	}()
	return stream, nil
}

// Generate drains Stream and returns its final result.
func (m *Model) Generate(ctx context.Context, req models.Request) (*models.Message, error) {
	return models.Generate(ctx, m, req)
}

// Info returns the selected model's metadata.
func (m *Model) Info() models.ModelInfo { return m.Info_ }

// LoweredOptions reports request flags calculated by the selected protocol.
func (m *Model) LoweredOptions(ctx context.Context, req models.Request) models.LoweredOptions {
	return m.Route.Protocol.LoweredOptions(req)
}

// CountTokens estimates text tokens using a local characters-per-token heuristic.
func (m *Model) CountTokens(ctx context.Context, msgs []models.Message) (int, error) {
	total := 0
	for _, msg := range msgs {
		for _, part := range msg.Content {
			if text, ok := part.(models.TextPart); ok {
				total += len(text.Text)
			}
		}
	}
	return total / 4, nil
}

func responseRequestID(headers http.Header) string {
	for _, name := range []string{"x-request-id", "request-id", "openai-request-id", "x-goog-request-id"} {
		if id := strings.TrimSpace(headers.Get(name)); id != "" {
			return id
		}
	}
	return ""
}

func mergeHeaders(layers ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, layer := range layers {
		for name, value := range layer {
			out[strings.ToLower(name)] = value
		}
	}
	return out
}

// Overlay merges a request overlay without mutating either input's nested maps.
func Overlay(base, patch map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(patch))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		if next, ok := v.(map[string]any); ok {
			previous, _ := out[k].(map[string]any)
			out[k] = Overlay(previous, next)
		} else {
			out[k] = v
		}
	}
	return out
}

// MergeStrings combines maps with later values taking precedence.
func MergeStrings(base, patch map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(patch))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		out[k] = v
	}
	return out
}
