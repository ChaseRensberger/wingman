package agent

import (
	"encoding/json"

	"github.com/chaserensberger/wingman/models"
	"github.com/invopop/jsonschema"
)

// SchemaFor reflects T into an Anthropic-compatible JSON Schema.
// Use as: agent.SchemaFor[MyResult]().
func SchemaFor[T any]() *models.OutputSchema {
	var zero T
	r := jsonschema.Reflector{
		AllowAdditionalProperties:  false,
		DoNotReference:             true,
		RequiredFromJSONSchemaTags: false,
	}
	s := r.Reflect(&zero)
	b, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	// invopop emits a top-level $schema key; strip it for cleaner wire form.
	delete(m, "$schema")
	delete(m, "$id")
	return &models.OutputSchema{Schema: m}
}
