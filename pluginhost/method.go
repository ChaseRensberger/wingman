package pluginhost

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Invoke validates and calls one declared custom RPC operation.
func (a *Activation) Invoke(ctx context.Context, pluginID, method string, input json.RawMessage, output *json.RawMessage) error {
	for _, p := range a.plugins {
		if p.id.ID == pluginID {
			for _, declaration := range p.contributions.Methods {
				if declaration.Name == method {
					if len(input) == 0 {
						input = json.RawMessage(`{}`)
					}
					if err := validateJSON(declaration.InputSchema, input); err != nil {
						return fmt.Errorf("plugin RPC input: %w", err)
					}
					if err := a.Call(ctx, pluginID, "rpc.execute", map[string]any{"name": method, "input": input}, output); err != nil {
						return err
					}
					if len(declaration.OutputSchema) != 0 {
						if err := validateJSON(declaration.OutputSchema, *output); err != nil {
							return fmt.Errorf("plugin RPC output: %w", err)
						}
					}
					return nil
				}
			}
		}
	}
	return fmt.Errorf("plugin RPC %s.%s is not registered", pluginID, method)
}

func validateJSON(schema map[string]any, raw json.RawMessage) error {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("plugin.json", schema); err != nil {
		return err
	}
	compiled, err := compiler.Compile("plugin.json")
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return compiled.Validate(value)
}
