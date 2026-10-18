package plugin

import (
	"context"
	"reflect"
	"testing"

	"github.com/chaserensberger/wingman/agent/run"
)

func TestCompletionValidatorsStopAtFirstContinuation(t *testing.T) {
	r := NewRegistry()
	var seen []int
	for i := range 3 {
		if err := r.RegisterBeforeComplete(func(context.Context, run.CompletionInfo) (run.CompletionDecision, error) {
			seen = append(seen, i)
			if i == 1 {
				return run.CompletionDecision{Action: "continue", Feedback: "fix"}, nil
			}
			return run.CompletionDecision{Action: "accept"}, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	built, _, err := r.build()
	if err != nil {
		t.Fatal(err)
	}
	d, err := built.Hooks.BeforeComplete(t.Context(), run.CompletionInfo{})
	if err != nil || d.Action != "continue" || !reflect.DeepEqual(seen, []int{0, 1}) {
		t.Fatalf("decision=%#v seen=%v err=%v", d, seen, err)
	}
}
