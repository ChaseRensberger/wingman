package plugin

import (
	"context"
	"encoding/json"

	"github.com/chaserensberger/wingman/plugin/protocol"
)

// NewMethod adapts a typed Go handler to a schema-validated custom operation.
func NewMethod[I, O any](declaration protocol.Method, handler func(context.Context, I) (O, error)) Method {
	return Method{Method: declaration, Execute: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var input I
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		return handler(ctx, input)
	}}
}

// CallPlugin invokes another plugin's declared operation in the same client scope.
func (h *Host) CallPlugin(ctx context.Context, pluginID, method string, input, output any) error {
	return h.Call(ctx, "plugin.call", map[string]any{"plugin": pluginID, "method": method, "input": input}, output)
}
