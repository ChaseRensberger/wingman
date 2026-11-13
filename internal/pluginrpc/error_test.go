package pluginrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/models"
)

func TestProviderErrorRoundTripExcludesPrivateDetails(t *testing.T) {
	delay := 3 * time.Second
	failure := &models.ProviderError{
		Category: models.ErrorRateLimit, Classification: "rate-limit", Provider: "native",
		Status: 429, RequestID: "req-native", Retryable: true, RetryAfter: &delay,
		Message: "rate limited", Metadata: map[string]string{"code": "throttled"},
		Cause: errors.New("private cause"), Diagnostic: &models.CallDiagnostic{},
	}
	raw, err := json.Marshal(NewError(fmt.Errorf("request failed: %w", failure)))
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private cause", "Cause", "Diagnostic"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("error included %q: %s", private, raw)
		}
	}
	var fault Error
	if err := json.Unmarshal(raw, &fault); err != nil {
		t.Fatal(err)
	}
	var restored *models.ProviderError
	if !errors.As(&fault, &restored) {
		t.Fatalf("provider failure was not restored: %v", &fault)
	}
	want := *failure
	want.Cause, want.Diagnostic = nil, nil
	if !reflect.DeepEqual(restored, &want) {
		t.Fatalf("restored failure = %#v, want %#v", restored, &want)
	}
}

func TestJoinedErrorsCannotBecomeRetryableProviderFailures(t *testing.T) {
	failure := &models.ProviderError{Category: models.ErrorUnavailable, Retryable: true}
	err := errors.Join(failure, errors.New("persistence failed"))
	fault := NewError(fmt.Errorf("combined failure: %w", err))
	var provider *models.ProviderError
	if errors.As(fault, &provider) || fault.Data != nil {
		t.Fatalf("joined failure became retryable: %#v", fault)
	}
}
