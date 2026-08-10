package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/chaserensberger/wingman/internal/daemonclient"
	"github.com/chaserensberger/wingman/internal/daemonstate"
)

var discoverManagedDaemon = func(ctx context.Context) (*daemonclient.Client, error) {
	stateDir, err := managedStateDir()
	if err != nil {
		return nil, err
	}
	return daemonclient.New(ctx, daemonstate.New(stateDir), version)
}

var runBrowserCommand = func(ctx context.Context, name, target string) error {
	return exec.CommandContext(ctx, name, target).Run()
}

func runConsole(ctx context.Context, cmd *cli.Command) error {
	client, err := discoverManagedDaemon(ctx)
	if err != nil {
		return err
	}
	consoleURL, err := resolveURL(client.URL(), "/console")
	if err != nil {
		return err
	}
	name, err := browserCommandName(runtime.GOOS)
	if err != nil {
		return err
	}
	if err := runBrowserCommand(ctx, name, consoleURL); err != nil {
		return fmt.Errorf("open console URL %s: %w", consoleURL, err)
	}
	return nil
}

func validatePublicURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("--url must be an absolute HTTP or HTTPS URL with a host: %q", raw)
	}
	if parsed.Scheme == "http" && !hostIsLoopback(parsed.Hostname()) {
		return nil, fmt.Errorf("--url must use HTTPS for a non-loopback host: %q", raw)
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

func resolveURL(base, pathValue string) (string, error) {
	parsedBase, err := validatePublicURL(base)
	if err != nil {
		return "", err
	}
	path, err := url.Parse(pathValue)
	if err != nil {
		return "", fmt.Errorf("parse URL path: %w", err)
	}
	return parsedBase.ResolveReference(path).String(), nil
}

func browserCommandName(goos string) (string, error) {
	switch goos {
	case "linux":
		return "xdg-open", nil
	case "darwin":
		return "open", nil
	default:
		return "", fmt.Errorf("wingman console supports Linux and macOS only")
	}
}

func commandWriter(cmd *cli.Command) io.Writer {
	if writer := cmd.Root().Writer; writer != nil {
		return writer
	}
	return os.Stdout
}
