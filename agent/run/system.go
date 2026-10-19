package run

import (
	"context"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/tool"
)

// TransformSystemInfo contains the current execution context for one model call.
type TransformSystemInfo struct {
	System      string
	Model       models.ModelRef
	ModelInfo   models.ModelInfo
	WorkDir     string
	Tools       []tool.Tool
	Permissions permission.Ruleset
}

// TransformSystemHook rewrites the effective system prompt before each model call.
type TransformSystemHook func(context.Context, TransformSystemInfo) (string, error)
