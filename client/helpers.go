package client

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chaserensberger/wingman/api"
	"github.com/segmentio/ksuid"
)

// RegisterClient creates an API client identity.
func (c *SDK) RegisterClient(ctx context.Context, id, name string) (Client, error) {
	response, err := c.CreateClientWithResponse(ctx, nil, CreateClientRequest{Id: id, Name: name})
	if err != nil {
		return Client{}, err
	}
	if response.JSON201 == nil {
		return Client{}, fmt.Errorf("register client returned HTTP %d without a client", response.StatusCode())
	}
	return response.JSON201.Client, nil
}

// EnsureClient creates a client identity or returns the matching existing client.
func (c *SDK) EnsureClient(ctx context.Context, id, name string) (Client, error) {
	id, name = strings.TrimSpace(id), strings.TrimSpace(name)
	created, err := c.RegisterClient(ctx, id, name)
	if err == nil {
		return created, nil
	}
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.Response.Code != api.ErrorCodeConflict {
		return Client{}, err
	}

	response, getErr := c.GetClientWithResponse(ctx, id, nil)
	if getErr != nil {
		return Client{}, getErr
	}
	if response.JSON200 == nil {
		return Client{}, fmt.Errorf("get client returned HTTP %d without a client", response.StatusCode())
	}
	existing := *response.JSON200
	if existing.Id != id || existing.Name != name {
		return Client{}, fmt.Errorf("client %q does not match the requested identity", id)
	}
	return existing, nil
}

// NewMessageAdmission assigns a request ID when one is not already present.
func NewMessageAdmission(request MessageSessionRequest) MessageSessionRequest {
	if request.RequestId == nil || *request.RequestId == "" {
		requestID := ksuid.New().String()
		request.RequestId = &requestID
	}
	return request
}

// AdmitMessage durably queues one persistent-session message.
func (c *SDK) AdmitMessage(ctx context.Context, sessionID string, request MessageSessionRequest) (MessageSessionResponse, error) {
	if request.RequestId == nil || *request.RequestId == "" {
		return MessageSessionResponse{}, errors.New("message admission requires a request ID; call NewMessageAdmission and persist its result before retrying")
	}
	response, err := c.MessageSessionWithResponse(ctx, sessionID, nil, request)
	if err != nil {
		return MessageSessionResponse{}, err
	}
	if response.JSON202 == nil {
		return MessageSessionResponse{}, fmt.Errorf("admit message returned HTTP %d without an admission", response.StatusCode())
	}
	return *response.JSON202, nil
}
