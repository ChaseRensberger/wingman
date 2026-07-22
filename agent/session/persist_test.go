package session

import (
	"context"
	"math"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestPersistModelCallStoresUsageUnavailableAndEstimatedCost(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_test"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	sess := New(WithID(stored.ID), WithStore(data))
	model := models.ModelRef{Provider: "test", ID: "model"}
	info := models.ModelInfo{InputCostPerMTok: 3, OutputCostPerMTok: 15}

	if err := sess.persistModelCall(context.Background(), "msg_zero", 1, models.Message{}, model, info, ""); err != nil {
		t.Fatal(err)
	}
	if err := sess.persistModelCall(context.Background(), "msg_used", 2, models.Message{
		Usage: &models.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
	}, model, info, ""); err != nil {
		t.Fatal(err)
	}

	calls, err := data.ListModelCalls(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("call count = %d, want 2", len(calls))
	}
	if calls[0].TotalTokens != 0 || calls[0].Cost != 0 {
		t.Fatalf("usage-unavailable call = %#v, want zero usage and cost", calls[0])
	}
	if math.Abs(calls[1].Cost-18) > 0.000001 {
		t.Fatalf("cost = %f, want 18", calls[1].Cost)
	}
}
