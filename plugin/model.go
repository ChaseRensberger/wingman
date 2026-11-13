package plugin

import (
	"context"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
)

type hostModelClient struct{ host *Host }

func (c *hostModelClient) Prepare(ctx context.Context, request models.Request) (*models.PreparedRequest, error) {
	var result models.PreparedRequest
	err := c.host.Call(ctx, "model.prepare", request, &result)
	return &result, err
}

func (c *hostModelClient) Generate(ctx context.Context, request models.Request) (*models.Message, error) {
	var result models.Message
	err := c.host.Call(ctx, "model.generate", request, &result)
	return &result, err
}

func (c *hostModelClient) Stream(ctx context.Context, request models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	stream := models.NewEventStream[models.StreamPart, *models.Message](1)
	stream.BindContext(ctx)
	go func() {
		message, err := c.Generate(ctx, request)
		if err == nil {
			stream.Push(models.FinishPart{Message: message, Reason: message.FinishReason})
		}
		stream.Close(message, err)
	}()
	return stream, nil
}

type hostSink struct {
	host *Host
	ctx  context.Context
}

func (s *hostSink) OnEvent(event run.Event) { _ = s.host.Call(s.ctx, "run.emit", event, nil) }
