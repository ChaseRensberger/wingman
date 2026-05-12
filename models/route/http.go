package route

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultMaxRetryDelay = 60 * time.Second

// HTTPTransport executes prepared JSON HTTP requests with bounded retries for
// transient transport failures, 429s, and 5xx responses.
type HTTPTransport struct {
	Client        *http.Client
	MaxRetries    int
	MaxRetryDelay time.Duration
}

func (t HTTPTransport) Do(ctx context.Context, prepared *PreparedRequest) (*http.Response, error) {
	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	maxDelay := t.MaxRetryDelay
	if maxDelay == 0 {
		maxDelay = defaultMaxRetryDelay
	}
	for attempt := 0; attempt <= t.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, prepared.Method, prepared.URL, bytes.NewReader(prepared.Body))
		if err != nil {
			return nil, err
		}
		req.Header = prepared.Headers.Clone()
		resp, err := client.Do(req)
		if err != nil {
			if shouldRetryTransport(err) && attempt < t.MaxRetries {
				if waitErr := waitForRetry(ctx, backoffDelay(attempt, maxDelay)); waitErr != nil {
					return nil, waitErr
				}
				continue
			}
			return nil, fmt.Errorf("request: %w", err)
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if shouldRetryStatus(resp.StatusCode) && attempt < t.MaxRetries {
			if waitErr := waitForRetry(ctx, retryDelay(resp, attempt, maxDelay)); waitErr != nil {
				return nil, fmt.Errorf("API %d: %s", resp.StatusCode, string(raw))
			}
			continue
		}
		return nil, fmt.Errorf("API %d: %s", resp.StatusCode, string(raw))
	}
	return nil, fmt.Errorf("retries exhausted")
}

func shouldRetryStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= http.StatusInternalServerError
}

func shouldRetryTransport(err error) bool {
	if err == nil {
		return false
	}
	return err != context.Canceled && err != context.DeadlineExceeded
}

func retryDelay(resp *http.Response, attempt int, maxDelay time.Duration) time.Duration {
	if resp != nil {
		if d, ok := parseRetryAfter(resp.Header.Get("Retry-After"), maxDelay); ok {
			return d
		}
	}
	return backoffDelay(attempt, maxDelay)
}

func parseRetryAfter(value string, maxDelay time.Duration) (time.Duration, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0, false
	}
	if seconds, err := time.ParseDuration(v + "s"); err == nil {
		if seconds < 0 {
			return 0, false
		}
		if seconds > maxDelay {
			seconds = maxDelay
		}
		return seconds, true
	}
	if t, err := time.Parse(time.RFC1123, v); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0, false
		}
		if d > maxDelay {
			d = maxDelay
		}
		return d, true
	}
	return 0, false
}

func backoffDelay(attempt int, maxDelay time.Duration) time.Duration {
	d := time.Second * time.Duration(2<<attempt)
	if d > maxDelay {
		return maxDelay
	}
	return d
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
