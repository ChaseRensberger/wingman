package pluginhost

import (
	"context"
	"fmt"
	"github.com/chaserensberger/wingman/plugin/protocol"
)

// AuthMethods returns discoverable methods from the captured generation.
func (a *Activation) AuthMethods() map[string][]protocol.AuthMethod {
	result := map[string][]protocol.AuthMethod{}
	for _, p := range a.plugins {
		result[p.id.ID] = append([]protocol.AuthMethod(nil), p.contributions.Auth...)
	}
	return result
}

// Authenticate invokes a declared method without exposing private results to API clients.
func (a *Activation) Authenticate(ctx context.Context, plugin string, call protocol.AuthCall) (protocol.AuthResult, error) {
	for _, method := range a.AuthMethods()[plugin] {
		if method.ID != call.Method {
			continue
		}
		switch call.Operation {
		case "start", "status", "complete", "cancel":
		default:
			return protocol.AuthResult{}, fmt.Errorf("invalid authentication operation")
		}
		call.Identity = a.identity
		var result protocol.AuthResult
		err := a.Call(ctx, plugin, "auth.execute", call, &result)
		return result, err
	}
	return protocol.AuthResult{}, fmt.Errorf("authentication method %q is not registered", call.Method)
}
