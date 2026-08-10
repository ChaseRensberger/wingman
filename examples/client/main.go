// Command client is a headless reference client for a remote Wingman server.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/api"
)

const requestTimeout = 10 * time.Second

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	if len(args) == 0 {
		_ = usage(output)
		return errors.New("command is required")
	}

	switch args[0] {
	case "connect":
		return connect(ctx, args[1:], output)
	case "status":
		return status(ctx, args[1:], output)
	case "disconnect":
		return disconnect(args[1:], output)
	case "help", "-h", "--help":
		return usage(output)
	default:
		_ = usage(output)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(output io.Writer) error {
	_, err := fmt.Fprintln(output, `Usage:
  client connect --server https://wingman.example --password <password> --password-file <path>
  client status --server https://wingman.example --password-file <path>
  client disconnect --password-file <path>`)
	return err
}

func connect(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("connect", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	server := flags.String("server", "", "Wingman server URL")
	password := flags.String("password", "", "Wingman password")
	passwordFile := flags.String("password-file", "", "Wingman password file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *password == "" || *passwordFile == "" {
		return errors.New("--password and --password-file are required")
	}

	baseURL, err := serverURL(*server)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: requestTimeout}
	ready, err := readiness(ctx, client, baseURL, *password)
	if err != nil {
		return err
	}
	if err := writePassword(*passwordFile, *password); err != nil {
		return err
	}
	fmt.Fprintf(output, "Connected to %s\nInstance: %s\nVersion: %s\n", baseURL, ready.InstanceID, ready.Version)
	return nil
}

func status(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	server := flags.String("server", "", "Wingman server URL")
	passwordFile := flags.String("password-file", "", "Wingman password file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *passwordFile == "" {
		return errors.New("--password-file is required")
	}
	baseURL, err := serverURL(*server)
	if err != nil {
		return err
	}
	password, err := readPassword(*passwordFile)
	if err != nil {
		return err
	}
	ready, err := readiness(ctx, &http.Client{Timeout: requestTimeout}, baseURL, password)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "Connected to %s\nInstance: %s\nVersion: %s\n", baseURL, ready.InstanceID, ready.Version)
	return nil
}

func disconnect(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("disconnect", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	passwordFile := flags.String("password-file", "", "Wingman password file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *passwordFile == "" {
		return errors.New("--password-file is required")
	}
	if err := os.Remove(*passwordFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove password file: %w", err)
	}
	fmt.Fprintln(output, "Removed local password.")
	return nil
}

func readiness(ctx context.Context, client *http.Client, baseURL *url.URL, password string) (api.ReadinessResponse, error) {
	var ready api.ReadinessResponse
	if err := requestJSON(ctx, client, baseURL, http.MethodGet, "/ready", password, nil, &ready); err != nil {
		return api.ReadinessResponse{}, err
	}
	if !ready.Ready {
		return api.ReadinessResponse{}, errors.New("Wingman is not ready")
	}
	return ready, nil
}

func requestJSON(ctx context.Context, client *http.Client, baseURL *url.URL, method, path, password string, requestBody, responseBody any) error {
	endpoint := baseURL.ResolveReference(&url.URL{Path: path})
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if password != "" {
		request.SetBasicAuth("wingman", password)
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request Wingman: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var failure api.ErrorResponse
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&failure); err == nil && failure.Error.Message != "" {
			return fmt.Errorf("Wingman returned HTTP %d: %s", response.StatusCode, failure.Error.Message)
		}
		return fmt.Errorf("Wingman returned HTTP %d", response.StatusCode)
	}
	if responseBody == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(responseBody); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func serverURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, fmt.Errorf("--server must be an origin URL: %q", raw)
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && hostIsLoopback(parsed.Hostname())) {
		return nil, fmt.Errorf("--server must use HTTPS for a non-loopback host: %q", raw)
	}
	return parsed, nil
}

func hostIsLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writePassword(path, password string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create password directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(password+"\n"), 0600); err != nil {
		return fmt.Errorf("write password file: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("secure password file: %w", err)
	}
	return nil
}

func readPassword(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read password file: %w", err)
	}
	password := strings.TrimSpace(string(data))
	if password == "" {
		return "", errors.New("password file is empty")
	}
	return password, nil
}
