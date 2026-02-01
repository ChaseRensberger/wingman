package errors

import (
	"errors"
	"testing"
)

func TestWingmanError(t *testing.T) {
	t.Run("error message without cause", func(t *testing.T) {
		err := New(ErrRateLimit, "too many requests")
		expected := "rate_limit: too many requests"
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("error message with cause", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := Wrap(ErrProviderUnavailable, "connection failed", cause)
		if err.Error() != "provider_unavailable: connection failed: underlying error" {
			t.Errorf("unexpected error message: %q", err.Error())
		}
	})

	t.Run("unwrap returns cause", func(t *testing.T) {
		cause := errors.New("cause")
		err := Wrap(ErrTimeout, "test", cause)
		if !errors.Is(err, cause) {
			t.Error("unwrap should return cause")
		}
	})
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		code      ErrorCode
		retryable bool
	}{
		{ErrRateLimit, true},
		{ErrProviderUnavailable, true},
		{ErrTimeout, true},
		{ErrToolFailed, false},
		{ErrPermissionDenied, false},
		{ErrInvalidInput, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			err := New(tt.code, "test")
			if IsRetryable(err) != tt.retryable {
				t.Errorf("expected retryable=%v for %s", tt.retryable, tt.code)
			}
		})
	}
}

func TestGetCode(t *testing.T) {
	err := New(ErrToolNotFound, "bash")
	if GetCode(err) != ErrToolNotFound {
		t.Errorf("expected ErrToolNotFound, got %s", GetCode(err))
	}

	stdErr := errors.New("standard error")
	if GetCode(stdErr) != "" {
		t.Error("expected empty code for standard error")
	}
}

func TestIs(t *testing.T) {
	err := RateLimit("too fast", nil)
	if !Is(err, ErrRateLimit) {
		t.Error("Is should return true for matching code")
	}
	if Is(err, ErrTimeout) {
		t.Error("Is should return false for non-matching code")
	}
}

func TestErrorHelpers(t *testing.T) {
	t.Run("ProviderUnavailable", func(t *testing.T) {
		err := ProviderUnavailable("test", nil)
		if err.Code != ErrProviderUnavailable {
			t.Error("wrong error code")
		}
	})

	t.Run("ToolFailed", func(t *testing.T) {
		err := ToolFailed("bash", errors.New("exit 1"))
		if err.Code != ErrToolFailed {
			t.Error("wrong error code")
		}
		if err.Message != "tool 'bash' failed" {
			t.Errorf("unexpected message: %s", err.Message)
		}
	})

	t.Run("ToolNotFound", func(t *testing.T) {
		err := ToolNotFound("unknown")
		if err.Code != ErrToolNotFound {
			t.Error("wrong error code")
		}
	})

	t.Run("MaxStepsExceeded", func(t *testing.T) {
		err := MaxStepsExceeded(50)
		if err.Code != ErrMaxStepsExceeded {
			t.Error("wrong error code")
		}
	})
}
