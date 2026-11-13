package plugin

import (
	"context"
	"errors"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

type toolDeclaration struct{ definition models.ToolDef }

func (t *toolDeclaration) Name() string        { return t.definition.Name }
func (t *toolDeclaration) Description() string { return t.definition.Description }
func (t *toolDeclaration) Definition() tool.Definition {
	return tool.Definition{Name: t.definition.Name, Description: t.definition.Description, RawInputSchema: t.definition.InputSchema}
}
func (t *toolDeclaration) Execute(context.Context, tool.Invocation) (tool.Result, error) {
	return tool.Result{}, errors.New("hook tool definitions are metadata; execute tools through a session")
}
