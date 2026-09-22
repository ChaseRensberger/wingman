package route

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/chaserensberger/wingman/models"
)

type cyclicDiagnosticError struct{}

func (*cyclicDiagnosticError) Error() string   { return "cyclic native error" }
func (e *cyclicDiagnosticError) Unwrap() error { return e }

func TestDiagnosticCauseBounds(t *testing.T) {
	var many []error
	for range 20 {
		many = append(many, errors.New(strings.Repeat("é", 2000)))
	}
	for _, err := range []error{&cyclicDiagnosticError{}, errors.Join(many...)} {
		truncated := false
		clean := func(s string, limit int) string {
			if len(s) <= limit {
				return s
			}
			truncated = true
			s = s[:limit]
			for !utf8.ValidString(s) {
				s = s[:len(s)-1]
			}
			return s
		}
		causes := diagnosticCauses(err, clean, &truncated)
		count := 0
		var visit func([]models.DiagnosticCause, int)
		visit = func(causes []models.DiagnosticCause, depth int) {
			for _, cause := range causes {
				count++
				if depth > 8 || len(cause.Message) > diagnosticFieldLimit || !utf8.ValidString(cause.Message) {
					t.Fatal("cause limits exceeded")
				}
				visit(cause.Causes, depth+1)
			}
		}
		visit(causes, 1)
		if !truncated || count > 16 {
			t.Fatalf("truncated=%v count=%d", truncated, count)
		}
	}
}

func TestDiagnosticHeaderBounds(t *testing.T) {
	for _, large := range []bool{false, true} {
		headers := http.Header{}
		for i := range 70 {
			value := "value"
			if large {
				value = strings.Repeat("é", 2049)
			}
			headers.Set(fmt.Sprintf("X-Provider-%03d", i), value)
		}
		failure := &models.ProviderError{Category: models.ErrorProvider}
		captureDiagnostic(failure, nil, nil, headers, "response", "", false)
		d := failure.Diagnostic
		bytes := 0
		for name, values := range d.ResponseHeaders {
			bytes += len(name)
			for _, value := range values {
				bytes += len(value)
				if !utf8.ValidString(value) || len(value) > diagnosticFieldLimit {
					t.Fatal("header value exceeds limit")
				}
			}
		}
		if !d.Truncated || len(d.ResponseHeaders) > 64 || bytes > 16<<10 {
			t.Fatalf("truncated=%v headers=%d bytes=%d", d.Truncated, len(d.ResponseHeaders), bytes)
		}
	}
}

func TestInvalidRequestSettingsRemainSerializable(t *testing.T) {
	prepared := &models.PreparedRequest{Body: map[string]any{"temperature": math.NaN(), "top_p": json.Number("invalid"), "generationConfig": map[string]any{"temperature": math.Inf(1)}}}
	_, err := (HTTP{}).Open(t.Context(), Route{}, prepared)
	var failure *models.ProviderError
	if !errors.As(err, &failure) || failure.Diagnostic == nil || failure.Diagnostic.Stage != "prepare" {
		t.Fatalf("failure = %v", err)
	}
	if _, err := json.Marshal(failure.Diagnostic); err != nil {
		t.Fatalf("diagnostic cannot be persisted: %v", err)
	}
}
