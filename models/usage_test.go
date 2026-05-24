package models

import "testing"

func TestUsageContextTokensAndPercent(t *testing.T) {
	usage := Usage{
		InputTokens:       100,
		OutputTokens:      20,
		ReasoningTokens:   5,
		CachedInputTokens: 10,
		CacheWriteTokens:  15,
	}

	if got := usage.ContextTokens(); got != 120 {
		t.Fatalf("ContextTokens() = %d, want 120", got)
	}
	if got := usage.ContextPercent(600); got != 20 {
		t.Fatalf("ContextPercent() = %v, want 20", got)
	}
}

func TestUsageDerivedTokenCountsClampAtZero(t *testing.T) {
	usage := Usage{
		InputTokens:       10,
		OutputTokens:      5,
		ReasoningTokens:   8,
		CachedInputTokens: 20,
		CacheWriteTokens:  5,
	}

	if got := usage.BillableInputTokens(); got != 0 {
		t.Fatalf("BillableInputTokens() = %d, want 0", got)
	}
	if got := usage.VisibleOutputTokens(); got != 0 {
		t.Fatalf("VisibleOutputTokens() = %d, want 0", got)
	}
	if got := usage.TotalOrComputed(); got != usage.ContextTokens() {
		t.Fatalf("TotalOrComputed() = %d, want computed context tokens", got)
	}
}
