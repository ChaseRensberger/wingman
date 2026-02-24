package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/agent"
	"github.com/chaserensberger/wingman/core"
	"github.com/chaserensberger/wingman/internal/storage"
	"github.com/chaserensberger/wingman/session"
	"gopkg.in/yaml.v3"
)

type formationDefinition struct {
	Name        string           `json:"name" yaml:"name"`
	Version     int              `json:"version" yaml:"version"`
	Description string           `json:"description,omitempty" yaml:"description,omitempty"`
	Defaults    formationDefault `json:"defaults,omitempty" yaml:"defaults,omitempty"`
	Nodes       []formationNode  `json:"nodes" yaml:"nodes"`
	Edges       []formationEdge  `json:"edges,omitempty" yaml:"edges,omitempty"`
}

type formationDefault struct {
	WorkDir string `json:"work_dir,omitempty" yaml:"work_dir,omitempty"`
}

type formationNode struct {
	ID    string                `json:"id" yaml:"id"`
	Kind  string                `json:"kind" yaml:"kind"`
	Role  string                `json:"role,omitempty" yaml:"role,omitempty"`
	Agent *formationAgentConfig `json:"agent,omitempty" yaml:"agent,omitempty"`
	Fleet *formationFleetConfig `json:"fleet,omitempty" yaml:"fleet,omitempty"`
}

type formationAgentConfig struct {
	Name         string         `json:"name,omitempty" yaml:"name,omitempty"`
	Provider     string         `json:"provider" yaml:"provider"`
	Model        string         `json:"model" yaml:"model"`
	Options      map[string]any `json:"options,omitempty" yaml:"options,omitempty"`
	Instructions string         `json:"instructions,omitempty" yaml:"instructions,omitempty"`
	Tools        []string       `json:"tools,omitempty" yaml:"tools,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty" yaml:"output_schema,omitempty"`
}

type formationFleetConfig struct {
	WorkerCount int                   `json:"worker_count,omitempty" yaml:"worker_count,omitempty"`
	FanoutFrom  string                `json:"fanout_from" yaml:"fanout_from"`
	TaskMapping map[string]string     `json:"task_mapping,omitempty" yaml:"task_mapping,omitempty"`
	Agent       *formationAgentConfig `json:"agent" yaml:"agent"`
}

type formationEdge struct {
	From string            `json:"from" yaml:"from"`
	To   string            `json:"to" yaml:"to"`
	When string            `json:"when,omitempty" yaml:"when,omitempty"`
	Map  map[string]string `json:"map,omitempty" yaml:"map,omitempty"`
}

type formationRunStats struct {
	NodesExecuted int   `json:"nodes_executed"`
	DurationMS    int64 `json:"duration_ms"`
}

type formationRunResult struct {
	Outputs map[string]map[string]any
	Stats   formationRunStats
}

type formationEvent struct {
	Type   string         `json:"type"`
	NodeID string         `json:"node_id,omitempty"`
	From   string         `json:"from,omitempty"`
	To     string         `json:"to,omitempty"`
	Count  int            `json:"count,omitempty"`
	Worker string         `json:"worker,omitempty"`
	Tool   string         `json:"tool,omitempty"`
	CallID string         `json:"call_id,omitempty"`
	Path   string         `json:"path,omitempty"`
	Output map[string]any `json:"output,omitempty"`
	Error  string         `json:"error,omitempty"`
	Status string         `json:"status,omitempty"`
	TS     string         `json:"ts"`
}

func decodeFormationDefinition(r *http.Request) (map[string]any, error) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, errors.New("request body is required")
	}

	raw := map[string]any{}
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "yaml") || strings.Contains(ct, "yml") {
		if err := yaml.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("invalid yaml body: %w", err)
		}
	} else {
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("invalid json body: %w", err)
		}
	}

	return normalizeDefinition(raw)
}

func normalizeDefinition(raw map[string]any) (map[string]any, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize definition: %w", err)
	}

	normalized := map[string]any{}
	if err := json.Unmarshal(b, &normalized); err != nil {
		return nil, fmt.Errorf("failed to normalize definition: %w", err)
	}

	return normalized, nil
}

func compileAndValidateDefinition(raw map[string]any) (*formationDefinition, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid definition: %w", err)
	}

	var def formationDefinition
	if err := json.Unmarshal(b, &def); err != nil {
		return nil, fmt.Errorf("invalid definition: %w", err)
	}
	if err := validateFormationDefinition(&def); err != nil {
		return nil, err
	}
	return &def, nil
}

func validateFormationDefinition(def *formationDefinition) error {
	if strings.TrimSpace(def.Name) == "" {
		return errors.New("name is required")
	}
	if def.Version == 0 {
		def.Version = 1
	}
	if len(def.Nodes) == 0 {
		return errors.New("nodes is required")
	}

	nodeSet := make(map[string]formationNode, len(def.Nodes))
	for _, node := range def.Nodes {
		if node.ID == "" {
			return errors.New("node id is required")
		}
		if _, exists := nodeSet[node.ID]; exists {
			return fmt.Errorf("duplicate node id: %s", node.ID)
		}
		nodeSet[node.ID] = node

		switch node.Kind {
		case "agent":
			if node.Agent == nil {
				return fmt.Errorf("node %q kind agent requires agent config", node.ID)
			}
			if node.Agent.Provider == "" || node.Agent.Model == "" {
				return fmt.Errorf("node %q requires agent provider and model", node.ID)
			}
			if node.Agent.OutputSchema == nil {
				return fmt.Errorf("node %q requires agent output_schema", node.ID)
			}
		case "fleet":
			if node.Fleet == nil {
				return fmt.Errorf("node %q kind fleet requires fleet config", node.ID)
			}
			if node.Fleet.Agent == nil {
				return fmt.Errorf("node %q fleet requires agent config", node.ID)
			}
			if node.Fleet.FanoutFrom == "" {
				return fmt.Errorf("node %q fleet requires fanout_from", node.ID)
			}
			if node.Fleet.Agent.Provider == "" || node.Fleet.Agent.Model == "" {
				return fmt.Errorf("node %q fleet agent requires provider and model", node.ID)
			}
			if node.Fleet.Agent.OutputSchema == nil {
				return fmt.Errorf("node %q fleet agent requires output_schema", node.ID)
			}
		case "join":
		default:
			return fmt.Errorf("node %q has unsupported kind %q", node.ID, node.Kind)
		}
	}

	for _, edge := range def.Edges {
		if edge.From == "" || edge.To == "" {
			return errors.New("edge from and to are required")
		}
		if _, ok := nodeSet[edge.From]; !ok {
			return fmt.Errorf("edge references unknown from node %q", edge.From)
		}
		if _, ok := nodeSet[edge.To]; !ok {
			return fmt.Errorf("edge references unknown to node %q", edge.To)
		}
	}

	if err := validateAcyclic(def); err != nil {
		return err
	}

	return nil
}

func validateAcyclic(def *formationDefinition) error {
	indegree := make(map[string]int, len(def.Nodes))
	adj := make(map[string][]string, len(def.Nodes))
	for _, n := range def.Nodes {
		indegree[n.ID] = 0
	}
	for _, e := range def.Edges {
		adj[e.From] = append(adj[e.From], e.To)
		indegree[e.To]++
	}

	queue := make([]string, 0, len(def.Nodes))
	for _, n := range def.Nodes {
		if indegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}

	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, to := range adj[id] {
			indegree[to]--
			if indegree[to] == 0 {
				queue = append(queue, to)
			}
		}
	}

	if visited != len(def.Nodes) {
		return errors.New("graph must be acyclic")
	}
	return nil
}

func (s *Server) runFormation(ctx context.Context, def *formationDefinition, inputs map[string]any, sink func(formationEvent)) (*formationRunResult, error) {
	start := time.Now()
	emit := func(e formationEvent) {
		e.TS = time.Now().UTC().Format(time.RFC3339)
		if sink != nil {
			sink(e)
		}
	}

	emit(formationEvent{Type: "run_start"})

	nodes := make(map[string]formationNode, len(def.Nodes))
	edgesFrom := make(map[string][]formationEdge, len(def.Nodes))
	remainingPred := make(map[string]int, len(def.Nodes))
	nodeInputs := make(map[string]map[string]any, len(def.Nodes))
	nodeOutputs := make(map[string]map[string]any, len(def.Nodes))

	for _, n := range def.Nodes {
		nodes[n.ID] = n
		remainingPred[n.ID] = 0
	}
	for _, e := range def.Edges {
		edgesFrom[e.From] = append(edgesFrom[e.From], e)
		remainingPred[e.To]++
	}

	queue := make([]string, 0, len(def.Nodes))
	for _, n := range def.Nodes {
		if remainingPred[n.ID] == 0 {
			queue = append(queue, n.ID)
			nodeInputs[n.ID] = cloneMap(inputs)
		}
	}
	sort.Strings(queue)

	executed := 0
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		node := nodes[nodeID]
		input := nodeInputs[nodeID]

		emit(formationEvent{Type: "node_start", NodeID: nodeID})

		out, err := s.executeFormationNode(ctx, def, node, input, nodeOutputs, emit)
		if err != nil {
			emit(formationEvent{Type: "node_error", NodeID: nodeID, Error: err.Error()})
			return nil, fmt.Errorf("node %s failed: %w", nodeID, err)
		}

		nodeOutputs[nodeID] = out
		executed++
		emit(formationEvent{Type: "node_output", NodeID: nodeID, Output: out})
		emit(formationEvent{Type: "node_end", NodeID: nodeID, Status: "ok"})

		for _, edge := range edgesFrom[nodeID] {
			mapped, ok := mapEdgePayload(edge, input, out, nodeOutputs)
			remainingPred[edge.To]--
			if ok {
				if nodeInputs[edge.To] == nil {
					nodeInputs[edge.To] = map[string]any{}
				}
				for k, v := range mapped {
					nodeInputs[edge.To][k] = v
				}
				emit(formationEvent{Type: "edge_emit", From: edge.From, To: edge.To, Count: 1})
			}

			if remainingPred[edge.To] == 0 {
				if _, hasInput := nodeInputs[edge.To]; hasInput {
					queue = append(queue, edge.To)
				}
			}
		}
	}

	stats := formationRunStats{NodesExecuted: executed, DurationMS: time.Since(start).Milliseconds()}
	emit(formationEvent{Type: "run_end", Status: "ok"})

	return &formationRunResult{Outputs: nodeOutputs, Stats: stats}, nil
}

func (s *Server) executeFormationNode(ctx context.Context, def *formationDefinition, node formationNode, input map[string]any, nodeOutputs map[string]map[string]any, emit func(formationEvent)) (map[string]any, error) {
	switch node.Kind {
	case "agent":
		return s.executeFormationAgentNode(ctx, def, node, input, emit)
	case "fleet":
		return s.executeFormationFleetNode(ctx, def, node, input, nodeOutputs, emit)
	case "join":
		return map[string]any{"status": "joined"}, nil
	default:
		return nil, fmt.Errorf("unsupported node kind: %s", node.Kind)
	}
}

func (s *Server) executeFormationAgentNode(ctx context.Context, def *formationDefinition, node formationNode, input map[string]any, emit func(formationEvent)) (map[string]any, error) {
	agentInstance, err := s.buildFormationAgent(node.Agent, node.ID)
	if err != nil {
		return nil, err
	}
	agentInstance = withSerializedEditTool(agentInstance)

	wd := def.Defaults.WorkDir
	runSession := session.New(session.WithAgent(agentInstance), session.WithWorkDir(wd))

	message := "{}"
	if len(input) > 0 {
		b, _ := json.Marshal(input)
		message = string(b)
	}
	if rawMessage, ok := input["message"].(string); ok && rawMessage != "" {
		message = rawMessage
	}

	runResult, err := runSessionWithToolEvents(ctx, runSession, message, node.ID, "", emit)
	if err != nil {
		return nil, err
	}

	if node.ID == "planner" && !runResult.writeStarted {
		retryPrompt := `You must call the write tool now.
Write non-empty markdown to ./report.md (title, table of contents, and section stubs), then return structured JSON only.
Do not skip the write tool call.`
		retryResult, retryErr := runSessionWithToolEvents(ctx, runSession, retryPrompt, node.ID, "", emit)
		if retryErr != nil {
			return nil, retryErr
		}
		runResult = mergeStreamedRunResults(runResult, retryResult)
	}

	if node.ID == "planner" {
		if !runResult.writeStarted {
			return nil, errors.New("planner must call write to create report.md")
		}
		if !runResult.writeCompleted {
			return nil, fmt.Errorf("planner started write tool call but did not complete it (likely truncated tool input or token limit). write attempts: %s", summarizeWriteAttempts(runResult.toolCalls))
		}
		if !runResult.writeExecuted {
			return nil, fmt.Errorf("planner completed write tool block but write did not execute successfully. write attempts: %s", summarizeWriteAttempts(runResult.toolCalls))
		}

		reportPath := filepath.Join(resolveWorkDir(def.Defaults.WorkDir), "report.md")
		reportBytes, readErr := os.ReadFile(reportPath)
		if readErr != nil {
			return nil, fmt.Errorf("planner did not produce report.md at %s: %w (write attempts: %s)", reportPath, readErr, summarizeWriteAttempts(runResult.toolCalls))
		}
		if strings.TrimSpace(string(reportBytes)) == "" {
			return nil, fmt.Errorf("planner produced empty report.md at %s (write attempts: %s)", reportPath, summarizeWriteAttempts(runResult.toolCalls))
		}
	}

	parsed, parseErr := parseStructuredObject(runResult.result.Response)
	if parseErr != nil {
		retryPrompt := "Your previous response was not valid JSON. Return ONLY valid JSON that matches the required output_schema. No prose, no markdown, no code fences."
		retryResult, retryErr := runSessionWithToolEvents(ctx, runSession, retryPrompt, node.ID, "", emit)
		if retryErr != nil {
			return nil, retryErr
		}
		runResult = mergeStreamedRunResults(runResult, retryResult)
		parsed, parseErr = parseStructuredObject(runResult.result.Response)
		if parseErr != nil {
			return nil, fmt.Errorf("node %q must return structured json output: %w (response preview: %s)", node.ID, parseErr, previewResponse(runResult.result.Response))
		}
	}

	return parsed, nil
}

func (s *Server) executeFormationFleetNode(ctx context.Context, def *formationDefinition, node formationNode, input map[string]any, nodeOutputs map[string]map[string]any, emit func(formationEvent)) (map[string]any, error) {
	items, err := resolveFanoutItems(node.Fleet.FanoutFrom, input, nodeOutputs)
	if err != nil {
		return nil, err
	}

	agentInstance, err := s.buildFormationAgent(node.Fleet.Agent, node.ID)
	if err != nil {
		return nil, err
	}
	agentInstance = withSerializedEditTool(agentInstance)

	messages := make([]string, 0, len(items))
	for _, item := range items {
		payload := map[string]any{}
		for key, expr := range node.Fleet.TaskMapping {
			payload[key] = evalItemExpr(expr, item)
		}
		if len(payload) == 0 {
			payload["item"] = item
		}
		b, _ := json.Marshal(payload)
		message := string(b)
		if rawMessage, ok := payload["message"].(string); ok && rawMessage != "" {
			message = rawMessage
		}
		messages = append(messages, message)
	}

	parsed, err := s.runFormationFleetWorkers(ctx, node.ID, agentInstance, def.Defaults.WorkDir, messages, node.Fleet.WorkerCount, emit)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"completed":        len(parsed),
		"results":          parsed,
		"all_workers_done": true,
	}, nil
}

type streamedRunResult struct {
	result         *session.Result
	writeStarted   bool
	writeCompleted bool
	writeExecuted  bool
	toolCalls      []formationToolCall
}

func mergeStreamedRunResults(a, b *streamedRunResult) *streamedRunResult {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return &streamedRunResult{
		result:         b.result,
		writeStarted:   a.writeStarted || b.writeStarted,
		writeCompleted: a.writeCompleted || b.writeCompleted,
		writeExecuted:  a.writeExecuted || b.writeExecuted,
		toolCalls:      append(a.toolCalls, b.toolCalls...),
	}
}

type formationToolCall struct {
	CallID string
	Tool   string
	Path   string
	Error  string
}

var formationEditMu sync.Mutex

type serializedEditTool struct {
	inner core.Tool
}

func (t *serializedEditTool) Name() string {
	return t.inner.Name()
}

func (t *serializedEditTool) Description() string {
	return t.inner.Description()
}

func (t *serializedEditTool) Definition() core.ToolDefinition {
	return t.inner.Definition()
}

func (t *serializedEditTool) Execute(ctx context.Context, params map[string]any, workDir string) (string, error) {
	formationEditMu.Lock()
	defer formationEditMu.Unlock()
	return t.inner.Execute(ctx, params, workDir)
}

func runSessionWithToolEvents(ctx context.Context, runSession *session.Session, message, nodeID, worker string, emit func(formationEvent)) (*streamedRunResult, error) {
	stream, err := runSession.RunStream(ctx, message)
	if err != nil {
		return nil, err
	}

	pending := map[int]core.StreamContentBlock{}
	callTools := map[string]string{}
	writeStarted := false
	writeCompleted := false
	perplexityCalls := 0
	for stream.Next() {
		event := stream.Event()
		if event.Type == core.EventContentBlockStart && event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
			pending[event.Index] = *event.ContentBlock
			callTools[event.ContentBlock.ID] = event.ContentBlock.Name
			if event.ContentBlock.Name == "perplexity_search" {
				perplexityCalls++
				if perplexityCalls > 3 {
					return nil, fmt.Errorf("agent exceeded max perplexity_search calls (3)")
				}
			}
			if event.ContentBlock.Name == "write" {
				writeStarted = true
			}
			emit(formationEvent{
				Type:   "tool_call",
				NodeID: nodeID,
				Worker: worker,
				Tool:   event.ContentBlock.Name,
				CallID: event.ContentBlock.ID,
				Status: "started",
			})
		}

		if event.Type == core.EventContentBlockStop {
			if block, ok := pending[event.Index]; ok {
				if block.Name == "write" {
					writeCompleted = true
				}
				emit(formationEvent{
					Type:   "tool_call",
					NodeID: nodeID,
					Worker: worker,
					Tool:   block.Name,
					CallID: block.ID,
					Status: "done",
				})
				delete(pending, event.Index)
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	toolCalls := make([]formationToolCall, 0, len(stream.Result().ToolCalls))
	writeExecuted := false
	for _, call := range stream.Result().ToolCalls {
		toolName := callTools[call.ToolName]
		path := extractToolPath(call.Input)
		callError := ""
		status := "done"
		if call.Error != nil {
			callError = call.Error.Error()
			status = "error"
		}

		toolCalls = append(toolCalls, formationToolCall{
			CallID: call.ToolName,
			Tool:   toolName,
			Path:   path,
			Error:  callError,
		})

		if toolName == "write" || toolName == "edit" {
			if call.Error == nil {
				if toolName == "write" {
					writeExecuted = true
				}
			} else if toolName == "edit" {
				return nil, fmt.Errorf("edit tool failed (path=%s): %w", path, call.Error)
			}
			emit(formationEvent{
				Type:   "tool_call",
				NodeID: nodeID,
				Worker: worker,
				Tool:   toolName,
				CallID: call.ToolName,
				Path:   path,
				Status: status,
				Error:  callError,
			})
		}
	}

	return &streamedRunResult{result: stream.Result(), writeStarted: writeStarted, writeCompleted: writeCompleted, writeExecuted: writeExecuted, toolCalls: toolCalls}, nil
}

func (s *Server) runFormationFleetWorkers(ctx context.Context, nodeID string, agentInstance *agent.Agent, workDir string, messages []string, maxWorkers int, emit func(formationEvent)) ([]map[string]any, error) {
	if len(messages) == 0 {
		return []map[string]any{}, nil
	}

	limit := maxWorkers
	if limit <= 0 || limit > len(messages) {
		limit = len(messages)
	}

	type result struct {
		index int
		data  map[string]any
		err   error
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan int)
	out := make(chan result, len(messages))

	var wg sync.WaitGroup
	for workerIdx := 0; workerIdx < limit; workerIdx++ {
		wg.Add(1)
		go func(workerNum int) {
			defer wg.Done()
			workerName := fmt.Sprintf("worker-%d", workerNum)
			for idx := range jobs {
				runSession := session.New(session.WithAgent(agentInstance), session.WithWorkDir(workDir))
				runResult, err := runSessionWithToolEvents(ctx, runSession, messages[idx], nodeID, workerName, emit)
				if err != nil {
					out <- result{index: idx, err: err}
					cancel()
					return
				}

				parsed, parseErr := parseStructuredObject(runResult.result.Response)
				if parseErr != nil {
					retryPrompt := "Your previous response was not valid JSON. Return ONLY valid JSON that matches the required output_schema. No prose, no markdown, no code fences."
					retryResult, retryErr := runSessionWithToolEvents(ctx, runSession, retryPrompt, nodeID, workerName, emit)
					if retryErr != nil {
						out <- result{index: idx, err: retryErr}
						cancel()
						return
					}
					runResult = mergeStreamedRunResults(runResult, retryResult)
					parsed, parseErr = parseStructuredObject(runResult.result.Response)
					if parseErr != nil {
						out <- result{index: idx, err: fmt.Errorf("fleet node %q must return structured json output: %w (response preview: %s)", nodeID, parseErr, previewResponse(runResult.result.Response))}
						cancel()
						return
					}
				}

				out <- result{index: idx, data: parsed}
			}
		}(workerIdx)
	}

	go func() {
		defer close(jobs)
		for i := range messages {
			select {
			case <-ctx.Done():
				return
			case jobs <- i:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(out)
	}()

	results := make([]map[string]any, len(messages))
	for item := range out {
		if item.err != nil {
			return nil, item.err
		}
		results[item.index] = item.data
	}

	final := make([]map[string]any, 0, len(results))
	for _, r := range results {
		if r != nil {
			final = append(final, r)
		}
	}

	return final, nil
}

func resolveWorkDir(workDir string) string {
	trimmed := strings.TrimSpace(workDir)
	if trimmed == "" {
		return "."
	}
	return trimmed
}

func withSerializedEditTool(a *agent.Agent) *agent.Agent {
	wrappedTools := make([]core.Tool, 0, len(a.Tools()))
	for _, t := range a.Tools() {
		if t.Name() == "edit" {
			wrappedTools = append(wrappedTools, &serializedEditTool{inner: t})
			continue
		}
		wrappedTools = append(wrappedTools, t)
	}

	return agent.New(
		a.Name(),
		agent.WithID(a.ID()),
		agent.WithInstructions(a.Instructions()),
		agent.WithProvider(a.Provider()),
		agent.WithProviderID(a.ProviderID()),
		agent.WithModel(a.Model()),
		agent.WithOutputSchema(a.OutputSchema()),
		agent.WithTools(wrappedTools...),
	)
}

func parseStructuredObject(raw string) (map[string]any, error) {
	parsed := map[string]any{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func previewResponse(raw string) string {
	trimmed := strings.TrimSpace(raw)
	const max = 220
	if len(trimmed) <= max {
		return trimmed
	}
	return trimmed[:max] + "..."
}

func validateReportArtifacts(reportPath string) error {
	b, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("failed to read report: %w", err)
	}
	content := string(b)
	issues := make([]string, 0)
	if strings.Contains(content, "<!-- SECTION:") {
		issues = append(issues, "section markers still present")
	}
	if strings.Contains(content, "TODO:") || strings.Contains(content, "_TODO:") {
		issues = append(issues, "TODO placeholders still present")
	}
	if strings.Contains(strings.ToLower(content), "<system-reminder>") {
		issues = append(issues, "system-reminder tag still present")
	}
	if len(issues) > 0 {
		return errors.New(strings.Join(issues, "; "))
	}
	return nil
}

func extractToolPath(input any) string {
	params, ok := input.(map[string]any)
	if !ok {
		return ""
	}
	path, _ := params["path"].(string)
	return path
}

func summarizeWriteAttempts(calls []formationToolCall) string {
	attempts := make([]string, 0)
	for _, call := range calls {
		if call.Tool != "write" {
			continue
		}
		path := call.Path
		if path == "" {
			path = "<no-path>"
		}
		if call.Error != "" {
			attempts = append(attempts, fmt.Sprintf("%s (error: %s)", path, call.Error))
			continue
		}
		attempts = append(attempts, fmt.Sprintf("%s (ok)", path))
	}

	if len(attempts) == 0 {
		return "none"
	}

	return strings.Join(attempts, "; ")
}

func (s *Server) buildFormationAgent(cfg *formationAgentConfig, fallbackName string) (*agent.Agent, error) {
	name := cfg.Name
	if name == "" {
		name = fallbackName
	}

	stored := &storage.Agent{
		Name:         name,
		Instructions: cfg.Instructions,
		Tools:        cfg.Tools,
		Provider:     cfg.Provider,
		Model:        cfg.Model,
		Options:      cfg.Options,
		OutputSchema: cfg.OutputSchema,
	}

	return s.buildAgent(stored)
}

func mapEdgePayload(edge formationEdge, input map[string]any, output map[string]any, outputs map[string]map[string]any) (map[string]any, bool) {
	if edge.When == "all_workers_done" {
		ready, _ := output["all_workers_done"].(bool)
		if !ready {
			return nil, false
		}
	}

	if len(edge.Map) == 0 {
		return cloneMap(output), true
	}

	mapped := map[string]any{}
	for toKey, expr := range edge.Map {
		mapped[toKey] = evalEdgeExpr(expr, input, output, outputs)
	}
	return mapped, true
}

func evalEdgeExpr(expr string, input map[string]any, output map[string]any, outputs map[string]map[string]any) any {
	expr = strings.TrimSpace(expr)
	if expr == "output" {
		return output
	}
	if expr == "input" {
		return input
	}
	if strings.HasPrefix(expr, "output.") {
		return getPath(output, strings.Split(strings.TrimPrefix(expr, "output."), "."))
	}
	if strings.HasPrefix(expr, "input.") {
		return getPath(input, strings.Split(strings.TrimPrefix(expr, "input."), "."))
	}

	parts := strings.Split(expr, ".")
	if len(parts) > 1 {
		if root, ok := outputs[parts[0]]; ok {
			return getPath(root, parts[1:])
		}
	}

	return expr
}

func resolveFanoutItems(fanout string, input map[string]any, outputs map[string]map[string]any) ([]any, error) {
	if strings.HasPrefix(fanout, "input.") {
		v := getPath(input, strings.Split(strings.TrimPrefix(fanout, "input."), "."))
		return castToSlice(v)
	}

	parts := strings.Split(fanout, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid fanout_from path: %s", fanout)
	}
	root, ok := outputs[parts[0]]
	if !ok {
		return nil, fmt.Errorf("fanout_from references unknown node output: %s", parts[0])
	}
	v := getPath(root, parts[1:])
	return castToSlice(v)
}

func castToSlice(v any) ([]any, error) {
	s, ok := v.([]any)
	if ok {
		return s, nil
	}
	return nil, errors.New("fanout value must be an array")
}

func evalItemExpr(expr string, item any) any {
	expr = strings.TrimSpace(expr)
	if expr == "item" {
		return item
	}
	if strings.HasPrefix(expr, "item.") {
		if m, ok := item.(map[string]any); ok {
			return getPath(m, strings.Split(strings.TrimPrefix(expr, "item."), "."))
		}
	}
	return expr
}

func getPath(root map[string]any, parts []string) any {
	var current any = root
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[part]
	}
	return current
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
