package models

import (
	"context"
	"fmt"
)

// Run drains a client stream synchronously and returns the assembled
// assistant message. It discards intermediate stream parts; callers that
// need streaming should use Client.Stream directly.
func Run(ctx context.Context, client Client, req Request) (*Message, error) {
	stream, err := client.Stream(ctx, req)
	if err != nil {
		return nil, err
	}
	for range stream.Iter() {
		// drain
	}
	msg, err := stream.Final()
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, fmt.Errorf("model returned nil message")
	}
	return msg, nil
}
