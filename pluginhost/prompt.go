package pluginhost

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/chaserensberger/wingman/plugin/protocol"
)

// PreparePrompt transforms a user message in plugin order without changing its owner.
func (a *Activation) PreparePrompt(ctx context.Context, requestID, message string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, p := range a.plugins {
		if !slices.Contains(p.contributions.Hooks, "prompt") {
			continue
		}
		var next string
		if err := a.Call(ctx, p.id.ID, "prompt.prepare", protocol.Prompt{Identity: a.identity, RequestID: requestID, Message: message}, &next); err != nil {
			return "", fmt.Errorf("plugin %q prompt: %w", p.id.ID, err)
		}
		if next == "" {
			return "", fmt.Errorf("plugin %q returned an empty prompt", p.id.ID)
		}
		message = next
	}
	return message, ctx.Err()
}
