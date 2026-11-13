package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
	"github.com/chaserensberger/wingman/plugin/protocol"
)

type httpBodyKey struct{}
type httpBodyLifetimeKey struct{}
type httpBodyGrantKey struct{}

type httpBodyGrant struct{ stop func() bool }

type remoteHTTPBody struct {
	activation *Activation
	plugin     string
	id         string
	ctx        context.Context
	source     io.ReadCloser
	pending    []byte
	eof        bool
	once       sync.Once
	err        error
	stop       func() bool
}

func (b *remoteHTTPBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(b.pending) == 0 && !b.eof {
		var chunk protocol.BodyChunk
		ctx := context.WithValue(b.ctx, httpBodyKey{}, b.source)
		if err := b.activation.Call(ctx, b.plugin, "http.body.read", protocol.BodyRead{ID: b.id}, &chunk); err != nil {
			return 0, err
		}
		if len(chunk.Data) > 32*1024 {
			return 0, errors.New("plugin HTTP body chunk exceeds 32 KiB")
		}
		b.pending, b.eof = chunk.Data, chunk.EOF
	}
	n := copy(p, b.pending)
	b.pending = b.pending[n:]
	if n == 0 && b.eof {
		return 0, io.EOF
	}
	return n, nil
}

func (b *remoteHTTPBody) Close() error {
	b.once.Do(func() {
		if b.stop != nil {
			b.stop()
		}
		if b.source != nil {
			b.err = b.source.Close()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		b.err = errors.Join(b.err, b.activation.Call(ctx, b.plugin, "http.body.close", protocol.BodyRead{ID: b.id}, nil))
	})
	return b.err
}

func (a *Activation) exchange(ctx context.Context, p *loadedPlugin, method string, input protocol.HTTPExchange, source io.ReadCloser) (protocol.HTTPExchange, io.ReadCloser, error) {
	input.Identity, input.HasBody = a.identity, source != nil
	if source == nil {
		source = http.NoBody
	}
	var output protocol.HTTPExchange
	bodyCtx := context.WithValue(ctx, httpBodyKey{}, source)
	callCtx, cancel := context.WithTimeout(context.WithValue(bodyCtx, httpBodyLifetimeKey{}, bodyCtx), 30*time.Second)
	defer cancel()
	if err := a.Call(callCtx, p.id.ID, method, input, &output); err != nil {
		return output, source, err
	}
	if !output.HasBody {
		if source != nil {
			_ = source.Close()
		}
		return output, http.NoBody, nil
	}
	if output.BodyID == "" {
		return output, source, nil
	}
	body := &remoteHTTPBody{activation: a, plugin: p.id.ID, id: output.BodyID, ctx: ctx, source: source}
	// Close idle replacements on cancellation too, even when no read is in flight.
	ready := make(chan struct{})
	body.stop = context.AfterFunc(ctx, func() { <-ready; _ = body.Close() })
	close(ready)
	return output, body, nil
}

// HTTPHooks binds native HTTP interceptors to this activation's executable owners.
func (a *Activation) HTTPHooks() route.HTTPHooks {
	return route.HTTPHooks{
		Request: func(ctx context.Context, model models.ModelRef, req *http.Request) (*http.Request, error) {
			for _, p := range a.plugins {
				if !slices.Contains(p.contributions.Hooks, "http.request") {
					continue
				}
				out, body, err := a.exchange(ctx, p, "http.request", protocol.HTTPExchange{Model: model, Method: req.Method, URL: req.URL.String(), Headers: req.Header, ContentLength: req.ContentLength}, req.Body)
				if err != nil {
					if req.Body != nil {
						_ = req.Body.Close()
					}
					return nil, err
				}
				next, err := http.NewRequestWithContext(ctx, out.Method, out.URL, body)
				if err != nil {
					_ = body.Close()
					return nil, err
				}
				if next.URL.Scheme != "http" && next.URL.Scheme != "https" {
					_ = body.Close()
					return nil, fmt.Errorf("HTTP hook returned a non-HTTP URL")
				}
				next.Header, next.ContentLength = out.Headers, out.ContentLength
				if out.HasBody && out.BodyID == "" {
					next.GetBody = req.GetBody
				}
				req = next
			}
			return req, nil
		},
		Response: func(ctx context.Context, model models.ModelRef, req *http.Request, resp *http.Response) (*http.Response, error) {
			for _, p := range a.plugins {
				if !slices.Contains(p.contributions.Hooks, "http.response") {
					continue
				}
				out, body, err := a.exchange(ctx, p, "http.response", protocol.HTTPExchange{Model: model, Method: req.Method, URL: req.URL.String(), Headers: resp.Header, Status: resp.StatusCode, ContentLength: resp.ContentLength}, resp.Body)
				if err != nil {
					_ = resp.Body.Close()
					return nil, err
				}
				if out.Status < 100 || out.Status > 599 {
					_ = body.Close()
					return nil, fmt.Errorf("HTTP hook returned invalid status %d", out.Status)
				}
				copy := *resp
				copy.Header, copy.StatusCode, copy.ContentLength, copy.Body = out.Headers, out.Status, out.ContentLength, body
				copy.Status = fmt.Sprintf("%d %s", out.Status, http.StatusText(out.Status))
				resp = &copy
			}
			return resp, nil
		},
	}
}

func readHTTPBody(ctx context.Context) (any, error) {
	body, _ := ctx.Value(httpBodyKey{}).(io.ReadCloser)
	if body == nil {
		return nil, errors.New("HTTP body is unavailable outside its owning exchange")
	}
	stop := context.AfterFunc(ctx, func() { _ = body.Close() })
	defer stop()
	data := make([]byte, 32*1024)
	n, err := body.Read(data)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return protocol.BodyChunk{Data: data[:n], EOF: errors.Is(err, io.EOF)}, nil
}

type httpModelClient struct {
	models.Client
	activation *Activation
}

// WrapHTTP attaches captured native HTTP hooks to all model calls made by a run.
func (a *Activation) WrapHTTP(client models.Client) models.Client {
	for _, p := range a.plugins {
		if slices.Contains(p.contributions.Hooks, "http.request") || slices.Contains(p.contributions.Hooks, "http.response") {
			return &httpModelClient{Client: client, activation: a}
		}
	}
	return client
}
func (c *httpModelClient) Stream(ctx context.Context, request models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	return c.Client.Stream(route.WithHTTPHooks(ctx, c.activation.HTTPHooks()), request)
}
func (c *httpModelClient) Generate(ctx context.Context, request models.Request) (*models.Message, error) {
	return models.Generate(ctx, c, request)
}

func (c *httpModelClient) LoweredOptions(ctx context.Context, request models.Request) models.LoweredOptions {
	if client, ok := c.Client.(interface {
		LoweredOptions(context.Context, models.Request) models.LoweredOptions
	}); ok {
		return client.LoweredOptions(ctx, request)
	}
	return models.LoweredOptions{}
}
