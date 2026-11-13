package plugin

import (
	"context"
	"errors"
	"fmt"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/tool"
)

func (e *Extension) hook(ctx context.Context, h protocol.Hook) (protocol.Hook, error) {
	var err error
	switch h.Name {
	case "turn_start":
		if e.Hooks.OnTurnStart == nil {
			break
		}
		return h, e.Hooks.OnTurnStart(ctx, h.Step)
	case "turn_end":
		if e.Hooks.OnTurnEnd == nil {
			break
		}
		return h, e.Hooks.OnTurnEnd(ctx, h.Step, h.Turn)
	case "before_run":
		if e.Hooks.BeforeRun == nil {
			break
		}
		h.Messages, err = e.Hooks.BeforeRun(ctx, h.Messages)
		return h, err
	case "transform_system":
		if e.Hooks.TransformSystem == nil {
			break
		}
		tools := make([]tool.Tool, 0, len(h.Tools))
		for _, definition := range h.Tools {
			tools = append(tools, &toolDeclaration{definition: definition})
		}
		h.System, err = e.Hooks.TransformSystem(ctx, run.TransformSystemInfo{System: h.System, WorkDir: h.Identity.WorkDir, Model: h.Model, ModelInfo: h.ModelInfo, Permissions: h.Permissions, Tools: tools})
		return h, err
	case "transform_context":
		if e.Hooks.TransformContext == nil {
			break
		}
		h.Messages, err = e.Hooks.TransformContext(ctx, run.TransformContextInfo{Step: h.Step, Messages: h.Messages, Model: h.Model, ModelInfo: h.ModelInfo})
		return h, err
	case "transform_history":
		if e.Hooks.TransformHistory == nil {
			break
		}
		h.Messages, err = e.Hooks.TransformHistory(ctx, run.TransformHistoryInfo{Step: h.Step, ContextOverflow: h.ContextOverflow, Messages: h.Messages, Model: h.Model, ModelInfo: h.ModelInfo, Request: h.Request, Usage: h.Usage, Client: &hostModelClient{host: e.host}, Sink: &hostSink{host: e.host, ctx: ctx}})
		return h, err
	case "transform_tools":
		if e.Hooks.TransformToolDefs == nil {
			break
		}
		h.Tools, err = e.Hooks.TransformToolDefs(ctx, run.TransformToolDefsInfo{Step: h.Step, Tools: h.Tools, Model: h.Model, ModelInfo: h.ModelInfo})
		return h, err
	case "transform_params":
		if e.Hooks.TransformParams == nil {
			break
		}
		result, err := e.Hooks.TransformParams(ctx, run.TransformParamsInfo{Step: h.Step, Model: h.Model, ModelInfo: h.ModelInfo, Params: h.Params})
		h.Params = result.Params
		return h, err
	case "before_tool":
		if e.Hooks.BeforeToolCall == nil {
			break
		}
		h.Call.Args, err = e.Hooks.BeforeToolCall(ctx, h.Call)
		if errors.Is(err, run.ErrSkipTool) {
			h.Skip = err.Error()
			err = nil
		}
		return h, err
	case "after_tool":
		if e.Hooks.AfterToolCall == nil {
			break
		}
		h.Result, err = e.Hooks.AfterToolCall(ctx, h.Call, h.Result)
		return h, err
	case "before_complete":
		if e.Hooks.BeforeComplete == nil || h.Candidate == nil {
			break
		}
		h.Decision, err = e.Hooks.BeforeComplete(ctx, run.CompletionInfo{SessionID: h.Identity.SessionID, RunID: h.Identity.RunID, Step: h.Step, Continuations: h.Continuations, Candidate: *h.Candidate, Messages: h.Messages, Usage: h.Usage})
		return h, err
	case "after_run":
		if e.Hooks.AfterRun == nil {
			break
		}
		var runErr error
		if h.Error != "" {
			runErr = errors.New(h.Error)
		}
		return h, e.Hooks.AfterRun(ctx, run.AfterRunInfo{Result: h.RunResult, Err: runErr})
	}
	return h, fmt.Errorf("unregistered hook %q", h.Name)
}
