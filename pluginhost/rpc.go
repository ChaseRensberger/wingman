package pluginhost

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

type rpcClient struct {
	cmd    *exec.Cmd
	enc    *json.Encoder
	dec    *json.Decoder
	stdin  io.WriteCloser
	callMu sync.Mutex
	nextID int64
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message"`
}

func startRPC(ctx context.Context, command []string) (*rpcClient, error) {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start plugin: %w", err)
	}
	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	return &rpcClient{
		cmd:   cmd,
		enc:   json.NewEncoder(stdin),
		dec:   json.NewDecoder(bufio.NewReader(stdout)),
		stdin: stdin,
	}, nil
}

func (c *rpcClient) call(ctx context.Context, method string, params any, out any) error {
	c.callMu.Lock()
	defer c.callMu.Unlock()

	c.nextID++
	id := c.nextID
	if err := c.enc.Encode(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		return fmt.Errorf("send plugin request: %w", err)
	}

	resCh := make(chan rpcResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		var res rpcResponse
		if err := c.dec.Decode(&res); err != nil {
			errCh <- err
			return
		}
		resCh <- res
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return fmt.Errorf("read plugin response: %w", err)
	case res := <-resCh:
		if res.ID != id {
			return fmt.Errorf("plugin response id mismatch: got %d want %d", res.ID, id)
		}
		if res.Error != nil {
			return fmt.Errorf("plugin error: %s", res.Error.Message)
		}
		if out == nil || len(res.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(res.Result, out); err != nil {
			return fmt.Errorf("decode plugin result: %w", err)
		}
		return nil
	}
}

func (c *rpcClient) close() error {
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return c.cmd.Wait()
}
