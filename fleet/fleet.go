// Package fleet provides a high-level concurrent work primitive. A Fleet runs
// one agent template against N tasks in parallel, collecting results as each
// worker finishes.
//
// Each task can override the template agent's instructions and working
// directory, but shares its provider, model, tools, and output schema. This
// makes fleets ideal for "fan-out" patterns: explore 4 directories with the
// same Explore agent, process 10 documents with the same Parser agent, etc.
//
// Usage (blocking):
//
//	f := fleet.New(fleet.Config{
//	    Agent: myAgent,
//	    Tasks: []fleet.Task{
//	        {Message: "Analyse /src/auth", WorkDir: "/src/auth"},
//	        {Message: "Analyse /src/api",  WorkDir: "/src/api"},
//	    },
//	})
//	results, err := f.Run(context.Background())
//
// Usage (streaming — results arrive as workers finish):
//
//	fs, err := f.RunStream(context.Background())
//	for fs.Next() {
//	    r := fs.Result()
//	    fmt.Printf("worker done: %s\n", r.Result.Response)
//	}
package fleet

import (
	"context"
	"fmt"
	"sync"

	"github.com/chaserensberger/wingman/agent"
	"github.com/chaserensberger/wingman/session"
)

// Task describes the work for a single fleet worker.
type Task struct {
	// Message is the prompt sent to the worker's session.
	Message string

	// WorkDir overrides the fleet's default working directory for this task.
	// If empty, the fleet's WorkDir is used.
	WorkDir string

	// Instructions overrides the template agent's instructions for this task.
	// If empty, the template agent's instructions are used unchanged.
	Instructions string

	// Data is arbitrary caller-supplied metadata passed through to FleetResult.
	// Wingman does not inspect this field.
	Data any
}

// FleetResult is the outcome of one worker completing its task.
type FleetResult struct {
	// TaskIndex is the zero-based position of this task in Config.Tasks.
	TaskIndex int

	// WorkerName is an internal identifier for the worker goroutine.
	WorkerName string

	// Result is the completed session result. Nil if Error is non-nil.
	Result *session.Result

	// Error is non-nil if the worker's session failed.
	Error error

	// Data is the passthrough value from Task.Data.
	Data any
}

// Config configures a Fleet.
type Config struct {
	// Agent is the template agent. Every worker uses a copy of this agent,
	// overriding only the fields specified in each Task.
	// Exactly one of Agent or AgentID must be set.
	Agent *agent.Agent

	// AgentID is used by the server path: the server resolves AgentID to a
	// live *agent.Agent before running. SDK callers should set Agent directly.
	AgentID string

	// Tasks is the list of work items to run concurrently.
	Tasks []Task

	// WorkDir is the default working directory for tool execution.
	// Individual Tasks can override this.
	WorkDir string

	// MaxWorkers caps the number of concurrently running workers.
	// Zero (default) means all tasks run concurrently.
	MaxWorkers int
}

// Fleet runs a set of tasks concurrently against a single agent template.
type Fleet struct {
	cfg Config
}

// New creates a Fleet from cfg. The fleet does not start running until Run or
// RunStream is called.
func New(cfg Config) *Fleet {
	return &Fleet{cfg: cfg}
}

// ============================================================
//  Blocking execution
// ============================================================

// Run executes all tasks and blocks until every worker has finished (or the
// context is cancelled). Results are returned in an unspecified order — use
// FleetResult.TaskIndex to correlate results with tasks.
func (f *Fleet) Run(ctx context.Context) ([]FleetResult, error) {
	if f.cfg.Agent == nil {
		return nil, fmt.Errorf("fleet: Agent must be set")
	}
	if len(f.cfg.Tasks) == 0 {
		return nil, nil
	}

	results := make([]FleetResult, len(f.cfg.Tasks))
	var wg sync.WaitGroup

	sem := makeSem(f.cfg.MaxWorkers, len(f.cfg.Tasks))

	for i, task := range f.cfg.Tasks {
		wg.Add(1)
		go func(idx int, t Task) {
			defer wg.Done()
			if sem != nil {
				sem <- struct{}{}
				defer func() { <-sem }()
			}
			results[idx] = f.runTask(ctx, idx, t)
		}(i, task)
	}

	wg.Wait()
	return results, nil
}

// ============================================================
//  Streaming execution
// ============================================================

// RunStream starts all workers and returns a FleetStream immediately. Results
// become available via FleetStream.Next() as workers complete. The stream is
// exhausted when all workers have finished.
func (f *Fleet) RunStream(ctx context.Context) (*FleetStream, error) {
	if f.cfg.Agent == nil {
		return nil, fmt.Errorf("fleet: Agent must be set")
	}

	resultsCh := make(chan FleetResult, len(f.cfg.Tasks))

	sem := makeSem(f.cfg.MaxWorkers, len(f.cfg.Tasks))

	var wg sync.WaitGroup
	for i, task := range f.cfg.Tasks {
		wg.Add(1)
		go func(idx int, t Task) {
			defer wg.Done()
			if sem != nil {
				sem <- struct{}{}
				defer func() { <-sem }()
			}
			resultsCh <- f.runTask(ctx, idx, t)
		}(i, task)
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	return &FleetStream{ch: resultsCh}, nil
}

// runTask executes a single task in a new session derived from the template agent.
func (f *Fleet) runTask(ctx context.Context, idx int, task Task) FleetResult {
	workerName := fmt.Sprintf("worker-%d", idx)

	// Determine working directory.
	workDir := f.cfg.WorkDir
	if task.WorkDir != "" {
		workDir = task.WorkDir
	}

	// Build the agent for this worker. If the task overrides instructions, we
	// need a modified copy of the template agent.
	a := f.cfg.Agent
	if task.Instructions != "" {
		// Create a new agent with overridden instructions but sharing the same
		// provider, tools, output schema, and model metadata.
		a = agent.New(f.cfg.Agent.Name(),
			agent.WithID(f.cfg.Agent.ID()),
			agent.WithInstructions(task.Instructions),
			agent.WithProvider(f.cfg.Agent.Provider()),
			agent.WithTools(f.cfg.Agent.Tools()...),
			agent.WithOutputSchema(f.cfg.Agent.OutputSchema()),
			agent.WithProviderID(f.cfg.Agent.ProviderID()),
			agent.WithModel(f.cfg.Agent.Model()),
		)
	}

	s := session.New(
		session.WithAgent(a),
		session.WithWorkDir(workDir),
	)

	result, err := s.Run(ctx, task.Message)
	return FleetResult{
		TaskIndex:  idx,
		WorkerName: workerName,
		Result:     result,
		Error:      err,
		Data:       task.Data,
	}
}

// ============================================================
//  FleetStream
// ============================================================

// FleetStream provides incremental access to fleet results as workers finish.
type FleetStream struct {
	ch      <-chan FleetResult
	current FleetResult
	results []FleetResult
}

// Next blocks until the next worker finishes. Returns false when all workers
// are done or the channel is closed.
func (fs *FleetStream) Next() bool {
	r, ok := <-fs.ch
	if !ok {
		return false
	}
	fs.current = r
	fs.results = append(fs.results, r)
	return true
}

// Result returns the most recently completed worker's result.
func (fs *FleetStream) Result() FleetResult {
	return fs.current
}

// Results returns all results collected so far.
func (fs *FleetStream) Results() []FleetResult {
	return fs.results
}

// ============================================================
//  Helpers
// ============================================================

// makeSem returns a buffered channel to use as a semaphore, or nil if there is
// no concurrency limit.
func makeSem(maxWorkers, taskCount int) chan struct{} {
	if maxWorkers <= 0 || maxWorkers >= taskCount {
		return nil
	}
	return make(chan struct{}, maxWorkers)
}
