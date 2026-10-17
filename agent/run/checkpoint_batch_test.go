package run

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

func TestCheckpointBatchFlushesIdleTrailingDelta(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stream := models.NewEventStream[models.StreamPart, *models.Message](0)
		var savedRevision atomic.Int64
		var savedText, delivered atomic.Int64
		done := make(chan error, 1)
		go func() {
			_, err := Run(t.Context(), Config{Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
				return stream, nil
			}}, Model: testModel,
				MessageCheckpoint: checkpointFunc(func(ctx context.Context, info MessageCheckpointInfo) (models.Message, error) {
					message, err := identifiedCheckpoint().Save(ctx, info)
					if len(message.Content) > 0 {
						savedText.Store(int64(len(message.Content[0].(models.TextPart).Text)))
					}
					savedRevision.Store(message.Revision)
					return message, err
				}), Sink: SinkFunc(func(event Event) {
					if e, ok := event.(StreamPartEvent); ok {
						if e.Revision > savedRevision.Load() {
							t.Error("event preceded durable checkpoint")
						}
						if _, ok := e.Part.(models.TextDeltaPart); ok {
							delivered.Add(1)
							if e.PartID != "part_1" {
								t.Error("unstable part ID")
							}
						}
					}
				}),
			})
			done <- err
		}()
		stream.Push(models.TextStartPart{ID: "text"})
		stream.Push(models.TextDeltaPart{ID: "text", Delta: "partial"})
		synctest.Wait()
		if savedText.Load() != 0 || delivered.Load() != 0 {
			t.Fatal("delta was not batched")
		}
		time.Sleep(99 * time.Millisecond)
		synctest.Wait()
		if delivered.Load() != 0 {
			t.Fatal("early flush")
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if savedText.Load() != 7 || delivered.Load() != 1 {
			t.Fatalf("saved=%d delivered=%d", savedText.Load(), delivered.Load())
		}
		stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "partial"}}}, nil)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

func TestCheckpointBatchSizeAndCountFlush(t *testing.T) {
	for _, tc := range []struct {
		name         string
		chunks, size int
	}{{"count", 64, 1}, {"bytes", 2, 8192}} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				stream := models.NewEventStream[models.StreamPart, *models.Message](0)
				var textBytes atomic.Int64
				done := make(chan error, 1)
				go func() {
					_, err := Run(t.Context(), Config{Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
						return stream, nil
					}}, Model: testModel, MessageCheckpoint: checkpointFunc(func(ctx context.Context, info MessageCheckpointInfo) (models.Message, error) {
						if len(info.Message.Content) > 0 {
							textBytes.Store(int64(len(info.Message.Content[0].(models.TextPart).Text)))
						}
						return identifiedCheckpoint().Save(ctx, info)
					})})
					done <- err
				}()
				stream.Push(models.TextStartPart{ID: "text"})
				for i := 0; i < tc.chunks; i++ {
					stream.Push(models.TextDeltaPart{ID: "text", Delta: strings.Repeat("x", tc.size)})
				}
				synctest.Wait()
				if textBytes.Load() != int64(tc.chunks*tc.size) {
					t.Fatalf("saved bytes=%d", textBytes.Load())
				}
				stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: strings.Repeat("x", tc.chunks*tc.size)}}}, nil)
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestFailedBatchDoesNotPublishUncommittedDeltas(t *testing.T) {
	stream := models.NewEventStream[models.StreamPart, *models.Message](3)
	stream.Push(models.TextStartPart{ID: "text"})
	stream.Push(models.TextDeltaPart{ID: "text", Delta: "partial"})
	stream.Push(models.TextEndPart{ID: "text"})
	stream.Close(&models.Message{Role: models.RoleAssistant}, nil)
	var delivered int
	result, err := Run(t.Context(), Config{Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		return stream, nil
	}}, Model: testModel,
		MessageCheckpoint: checkpointFunc(func(ctx context.Context, info MessageCheckpointInfo) (models.Message, error) {
			if len(info.Message.Content) > 0 && info.Message.Content[0].(models.TextPart).Text != "" && info.Message.State != models.MessageStateFailed {
				return models.Message{}, errors.New("checkpoint failed")
			}
			return identifiedCheckpoint().Save(ctx, info)
		}), Sink: SinkFunc(func(event Event) {
			if e, ok := event.(StreamPartEvent); ok {
				if _, ok := e.Part.(models.TextDeltaPart); ok {
					delivered++
				}
			}
		}),
	})
	if err == nil || delivered != 0 || len(result.Messages) != 1 || result.Messages[0].Content[0].(models.TextPart).Text != "partial" {
		t.Fatalf("result=%#v err=%v delivered=%d", result, err, delivered)
	}
}

func TestToolInputBatchCheckpointPrecedesExecution(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved", true: "failed"}[fail], func(t *testing.T) {
			stream := models.NewEventStream[models.StreamPart, *models.Message](4)
			stream.Push(models.ToolInputStartPart{ID: "call", ToolName: "check"})
			stream.Push(models.ToolInputDeltaPart{ID: "call", Delta: `{"x":`})
			stream.Push(models.ToolInputDeltaPart{ID: "call", Delta: `1}`})
			stream.Push(models.ToolCallPart_{ID: "call", ToolName: "check", Input: map[string]any{"x": float64(1)}})
			stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call", Name: "check", Input: map[string]any{"x": float64(1)}}}}, nil)
			var saved models.ToolPart
			executions := 0
			_, err := Run(t.Context(), Config{Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
				return stream, nil
			}}, Model: testModel, MaxSteps: 1,
				MessageCheckpoint: checkpointFunc(func(ctx context.Context, info MessageCheckpointInfo) (models.Message, error) {
					message, err := identifiedCheckpoint().Save(ctx, info)
					if len(message.Content) > 0 {
						part := message.Content[0].(models.ToolPart)
						if fail && part.InputRaw != "" {
							return models.Message{}, errors.New("batch save failed")
						}
						saved = part
					}
					return message, err
				}), Tools: []tool.Tool{tool.NewFuncTool("check", "check", tool.Definition{Name: "check", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
					executions++
					if saved.InputRaw != `{"x":1}` || saved.Input["x"] != float64(1) || saved.ID != "part_1" {
						t.Errorf("uncommitted tool input: %#v", saved)
					}
					return tool.Result{}, nil
				})},
			})
			if (err != nil) != fail || (executions == 0) != fail {
				t.Fatalf("err=%v executions=%d", err, executions)
			}
		})
	}
}

func TestCanceledBatchRetainsPartialSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	go func() {
		stream.Push(models.TextStartPart{ID: "text"})
		stream.Push(models.TextDeltaPart{ID: "text", Delta: "partial"})
		cancel()
		stream.Close(nil, context.Canceled)
	}()
	var failed models.Message
	result, err := Run(ctx, Config{Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		return stream, nil
	}}, Model: testModel,
		MessageCheckpoint: checkpointFunc(func(ctx context.Context, info MessageCheckpointInfo) (models.Message, error) {
			if err := ctx.Err(); err != nil {
				return models.Message{}, err
			}
			if info.Message.State == models.MessageStateFailed {
				failed = info.Message
			}
			return identifiedCheckpoint().Save(ctx, info)
		}),
	})
	if !errors.Is(err, context.Canceled) || result.StopReason != StopReasonAborted || len(failed.Content) != 1 || failed.Content[0].(models.TextPart).Text != "partial" {
		t.Fatalf("result=%#v err=%v failed=%#v", result, err, failed)
	}
}
