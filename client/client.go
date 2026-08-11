package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/api"
)

// SDK is an authenticated client for a Wingman daemon.
type SDK struct {
	*GeneratedClient
	*ClientWithResponses
	baseURL          *url.URL
	doer             *authenticatedDoer
	maxSSEEventBytes int
}

// Option configures an SDK.
type Option func(*clientConfig)

type clientConfig struct {
	doer             HttpRequestDoer
	password         string
	clientID         string
	maxSSEEventBytes int
}

// WithPassword configures the daemon password for HTTP Basic authentication.
func WithPassword(password string) Option {
	return func(config *clientConfig) {
		config.password = password
	}
}

// WithClientID sets X-Wingman-Client on every request.
func WithClientID(clientID string) Option {
	return func(config *clientConfig) {
		config.clientID = clientID
	}
}

// WithTransport sets the HTTP transport used for requests.
func WithTransport(doer HttpRequestDoer) Option {
	return func(config *clientConfig) {
		config.doer = doer
	}
}

// WithMaxSSEEventBytes sets the largest accepted server-sent event frame.
func WithMaxSSEEventBytes(size int) Option {
	return func(config *clientConfig) {
		config.maxSSEEventBytes = size
	}
}

// New creates an SDK for baseURL.
func New(baseURL string, options ...Option) (*SDK, error) {
	endpoint, err := url.Parse(baseURL)
	if err != nil || !endpoint.IsAbs() || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Path != "" && endpoint.Path != "/" {
		return nil, fmt.Errorf("base URL must be an origin URL: %q", baseURL)
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("base URL must use HTTP or HTTPS: %q", baseURL)
	}

	config := clientConfig{doer: http.DefaultClient, maxSSEEventBytes: 1 << 20}
	for _, option := range options {
		option(&config)
	}
	if config.doer == nil {
		config.doer = http.DefaultClient
	}
	if config.maxSSEEventBytes <= 0 {
		return nil, errors.New("maximum SSE event size must be positive")
	}

	doer := &authenticatedDoer{next: config.doer, password: config.password, clientID: config.clientID}
	generated, err := NewClient(strings.TrimRight(baseURL, "/"), WithHTTPClient(doer))
	if err != nil {
		return nil, fmt.Errorf("create generated client: %w", err)
	}
	return &SDK{
		GeneratedClient:     generated,
		ClientWithResponses: &ClientWithResponses{ClientInterface: generated},
		baseURL:             endpoint,
		doer:                doer,
		maxSSEEventBytes:    config.maxSSEEventBytes,
	}, nil
}

// APIError reports a non-success response from the Wingman API.
type APIError struct {
	StatusCode int
	Response   api.Error
	Headers    http.Header
	RequestID  string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	message := strings.TrimSpace(e.Response.Message)
	if message == "" {
		message = http.StatusText(e.StatusCode)
	}
	if e.Response.Code != "" {
		return fmt.Sprintf("Wingman API returned HTTP %d (%s): %s", e.StatusCode, e.Response.Code, message)
	}
	return fmt.Sprintf("Wingman API returned HTTP %d: %s", e.StatusCode, message)
}

type authenticatedDoer struct {
	next     HttpRequestDoer
	password string
	clientID string
}

func (d *authenticatedDoer) Do(request *http.Request) (*http.Response, error) {
	if d.password != "" {
		request.SetBasicAuth("wingman", d.password)
	}
	if d.clientID != "" {
		if request.Header.Get("X-Wingman-Client") == "" {
			request.Header.Set("X-Wingman-Client", d.clientID)
		}
	}

	response, err := d.next.Do(request)
	if err != nil || response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return response, err
	}
	defer response.Body.Close()

	failure := api.ErrorResponse{}
	apiError := &APIError{StatusCode: response.StatusCode, Headers: response.Header.Clone(), RequestID: response.Header.Get("X-Request-ID"), RetryAfter: retryAfter(response.Header.Get("Retry-After"), time.Now())}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&failure); err != nil {
		return nil, apiError
	}
	apiError.Response = failure.Error
	if apiError.RequestID == "" {
		apiError.RequestID = failure.Error.RequestID
	}
	return nil, apiError
}

func retryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}
