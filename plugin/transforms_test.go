package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chaserensberger/wingman/plugin/protocol"
)

func TestTransformsReplayAndDispose(t *testing.T) {
	transforms := &Transforms{}
	first := transforms.Agents(func(_ context.Context, agents protocol.Agents) error {
		agent := agents["agent"]
		agent.Instructions += " first"
		agents["agent"] = agent
		return nil
	})
	transforms.Agents(func(_ context.Context, agents protocol.Agents) error {
		agent := agents["agent"]
		agent.Instructions += " second"
		agents["agent"] = agent
		return nil
	})
	read := func() protocol.Agents {
		t.Helper()
		out, err := transforms.apply(context.Background(), protocol.TransformCall{Domain: "agents", Value: json.RawMessage(`{"agent":{"id":"agent","instructions":"base"}}`)})
		if err != nil {
			t.Fatal(err)
		}
		var result protocol.Agents
		if err := json.Unmarshal(out, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	captured := read()
	if got := read()["agent"].Instructions; got != "base first second" {
		t.Fatal(got)
	}
	first.Dispose()
	first.Dispose()
	if got := read()["agent"].Instructions; got != "base second" {
		t.Fatal(got)
	}
	if got := captured["agent"].Instructions; got != "base first second" {
		t.Fatal(got)
	}
}
