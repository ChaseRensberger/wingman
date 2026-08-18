package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v3"
)

type apiCall struct {
	method  string
	path    string
	body    string
	headers http.Header
}

type fakeRawDaemonClient struct {
	calls     []apiCall
	responses []*http.Response
}

func (f *fakeRawDaemonClient) Do(_ context.Context, method, path string, body io.Reader, headers http.Header) (*http.Response, error) {
	var contents []byte
	if body != nil {
		var err error
		contents, err = io.ReadAll(body)
		if err != nil {
			return nil, err
		}
	}
	f.calls = append(f.calls, apiCall{method: method, path: path, body: string(contents), headers: headers.Clone()})
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

func TestResolveAPIOperation(t *testing.T) {
	document := openAPIDocument{Paths: map[string]map[string]openAPIOperation{
		"/sessions/{id}/events": {"get": {OperationID: "streamSessionEvents"}},
	}}
	request, err := resolveAPIOperation(document, "streamSessionEvents", map[string]string{"id": "ses/a", "after": "12"})
	if err != nil {
		t.Fatal(err)
	}
	if request.Method != http.MethodGet || request.Path != "/sessions/ses%2Fa/events?after=12" {
		t.Fatalf("request = %#v", request)
	}
}

func TestResolveAPIOperationRequiresPathParameter(t *testing.T) {
	document := openAPIDocument{Paths: map[string]map[string]openAPIOperation{
		"/sessions/{id}": {"get": {OperationID: "getSession"}},
	}}
	_, err := resolveAPIOperation(document, "getSession", nil)
	if err == nil || !strings.Contains(err.Error(), "missing path parameter: id") {
		t.Fatalf("error = %v", err)
	}
}

func TestAPICommandResolvesOperationAndSendsRequest(t *testing.T) {
	client := &fakeRawDaemonClient{responses: []*http.Response{
		response(http.StatusOK, `{"paths":{"/sessions/{id}/message":{"post":{"operationId":"messageSession"}}}}`),
		response(http.StatusAccepted, `{"status":"queued"}`),
	}}
	var output bytes.Buffer
	cmd := apiCommand()
	cmd.Writer = &output
	cmd.Action = func(ctx context.Context, cmd *cli.Command) error {
		return runAPIWithClient(ctx, cmd, client)
	}
	err := cmd.Run(context.Background(), []string{"api", "messageSession", "--param", "id=ses_1", "--param", "after=12", "-d", `{"message":"hello"}`, "-H", "X-Wingman-Client:cli_test", "-H", "Accept:text/plain, application/json"})
	if err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != `{"status":"queued"}` {
		t.Fatalf("output = %q", got)
	}
	if len(client.calls) != 2 {
		t.Fatalf("calls = %#v", client.calls)
	}
	call := client.calls[1]
	if call.method != http.MethodPost || call.path != "/sessions/ses_1/message?after=12" || call.body != `{"message":"hello"}` {
		t.Fatalf("request = %#v", call)
	}
	if call.headers.Get("Content-Type") != "application/json" || call.headers.Get("X-Wingman-Client") != "cli_test" {
		t.Fatalf("headers = %#v", call.headers)
	}
	if call.headers.Get("Accept") != "text/plain, application/json" {
		t.Fatalf("Accept = %q", call.headers.Get("Accept"))
	}
}

func TestAPICommandReturnsHTTPErrorAfterWritingBody(t *testing.T) {
	client := &fakeRawDaemonClient{responses: []*http.Response{response(http.StatusConflict, `{"error":"conflict"}`)}}
	var output bytes.Buffer
	cmd := apiCommand()
	cmd.Writer = &output
	cmd.Action = func(ctx context.Context, cmd *cli.Command) error {
		return runAPIWithClient(ctx, cmd, client)
	}
	err := cmd.Run(context.Background(), []string{"api", "delete", "/sessions/ses_1"})
	if err == nil || !strings.Contains(err.Error(), "409 Conflict") {
		t.Fatalf("error = %v", err)
	}
	if output.String() != `{"error":"conflict"}` {
		t.Fatalf("output = %q", output.String())
	}
}

func TestAPICommandStreamsResponse(t *testing.T) {
	reader, writer := io.Pipe()
	client := &fakeRawDaemonClient{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       reader,
	}}}
	firstWrite := make(chan struct{}, 1)
	output := &notifyingWriter{written: firstWrite}
	cmd := apiCommand()
	cmd.Writer = output
	cmd.Action = func(ctx context.Context, cmd *cli.Command) error {
		return runAPIWithClient(ctx, cmd, client)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run(context.Background(), []string{"api", "get", "/sessions/ses_1/events"})
	}()
	writeDone := make(chan error, 1)
	go func() {
		_, err := io.WriteString(writer, "event: session.run.started\n\n")
		writeDone <- err
	}()
	select {
	case <-firstWrite:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for streamed output")
	}
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out writing streamed response")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for API command")
	}
	if output.String() != "event: session.run.started\n\n" {
		t.Fatalf("output = %q", output.String())
	}
}

type notifyingWriter struct {
	buffer  bytes.Buffer
	written chan<- struct{}
}

func (w *notifyingWriter) Write(value []byte) (int, error) {
	n, err := w.buffer.Write(value)
	select {
	case w.written <- struct{}{}:
	default:
	}
	return n, err
}

func (w *notifyingWriter) String() string {
	return w.buffer.String()
}

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)), Body: io.NopCloser(strings.NewReader(body))}
}
