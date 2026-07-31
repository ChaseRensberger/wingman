package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func TestSessionSummaryAndDetailHaveDistinctShapes(t *testing.T) {
	summary := Session{ID: "ses_test", Version: 3, CreatedAt: "created", UpdatedAt: "updated"}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "history") || strings.Contains(string(encoded), "latest_model_call") {
		t.Fatalf("summary contains detail fields: %s", encoded)
	}

	detail := SessionDetail{Session: summary, History: []models.Message{}}
	encoded, err = json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"history":[]`) {
		t.Fatalf("detail does not contain history: %s", encoded)
	}
}
