package fleet

import (
	"context"
	"fmt"
	"sync"

	"github.com/chaserensberger/wingman/agent"
	"github.com/chaserensberger/wingman/session"
)

type Task struct {
	Message      string
	WorkDir      string
	Instructions string
	Data         any
}

type FleetResult struct {
	TaskIndex  int
	WorkerName string
	Result     *session.Result
	Error      error
	Data       any
}

type Config struct {
	Agent      *agent.Agent
	AgentID    string
	Tasks      []Task
	WorkDir    string
	MaxWorkers int
}

type Fleet struct {
	cfg Config
}

func New(cfg Config) *Fleet {
	return &Fleet{cfg: cfg}
}

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

func (f *Fleet) runTask(ctx context.Context, idx int, task Task) FleetResult {
	workerName := fmt.Sprintf("worker-%d", idx)

	workDir := f.cfg.WorkDir
	if task.WorkDir != "" {
		workDir = task.WorkDir
	}

	a := f.cfg.Agent
	if task.Instructions != "" {
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

type FleetStream struct {
	ch      <-chan FleetResult
	current FleetResult
	results []FleetResult
}

func (fs *FleetStream) Next() bool {
	r, ok := <-fs.ch
	if !ok {
		return false
	}
	fs.current = r
	fs.results = append(fs.results, r)
	return true
}

func (fs *FleetStream) Result() FleetResult {
	return fs.current
}

func (fs *FleetStream) Results() []FleetResult {
	return fs.results
}

func makeSem(maxWorkers, taskCount int) chan struct{} {
	if maxWorkers <= 0 || maxWorkers >= taskCount {
		return nil
	}
	return make(chan struct{}, maxWorkers)
}
