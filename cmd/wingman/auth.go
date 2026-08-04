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
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/chaserensberger/wingman/api"
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

func authCommand() *cli.Command {
	return &cli.Command{
		Name:  "auth",
		Usage: "Manage console authorization",
		Commands: []*cli.Command{
			{
				Name:   "pair",
				Usage:  "Create a console pairing link",
				Flags:  authPairFlags(),
				Action: runAuthPair,
			},
			{
				Name:   "sessions",
				Usage:  "List authorization sessions",
				Flags:  authSessionsFlags(),
				Action: runAuthSessions,
			},
			{
				Name:      "revoke",
				Usage:     "Revoke an authorization session",
				ArgsUsage: "<session-id>",
				Action:    runAuthRevoke,
			},
		},
	}
}

func authPairFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "client", Usage: "Client identity (defaults to the default client)"},
		&cli.StringFlag{Name: "url", Usage: "Public console base URL"},
	}
}

func authSessionsFlags() []cli.Flag {
	return []cli.Flag{&cli.StringFlag{Name: "client", Usage: "Filter by client identity"}}
}

func runAuthPair(ctx context.Context, cmd *cli.Command) error {
	baseURL := cmd.String("url")
	if baseURL != "" {
		if _, err := validatePublicURL(baseURL); err != nil {
			return err
		}
	}
	client, err := discoverManagedDaemon(ctx)
	if err != nil {
		return err
	}
	var pairing api.PairingResponse
	if err := client.DoJSON(ctx, "POST", "/auth/pairings", api.CreatePairingRequest{ClientID: cmd.String("client")}, &pairing); err != nil {
		return err
	}
	if baseURL == "" {
		baseURL = client.URL()
	}
	pairingURL, err := resolvePairingURL(baseURL, pairing.PairingPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(commandWriter(cmd), "Pairing link valid until %s\n%s\n", pairing.ExpiresAt, pairingURL)
	return nil
}

func runAuthSessions(ctx context.Context, cmd *cli.Command) error {
	client, err := discoverManagedDaemon(ctx)
	if err != nil {
		return err
	}
	path := "/auth/sessions"
	if clientID := cmd.String("client"); clientID != "" {
		path += "?client_id=" + url.QueryEscape(clientID)
	}
	var sessions []api.AuthSession
	if err := client.DoJSON(ctx, "GET", path, nil, &sessions); err != nil {
		return err
	}
	writeAuthSessions(commandWriter(cmd), sessions)
	return nil
}

func runAuthRevoke(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 1 || strings.TrimSpace(cmd.Args().First()) == "" {
		return fmt.Errorf("expected exactly one session ID")
	}
	client, err := discoverManagedDaemon(ctx)
	if err != nil {
		return err
	}
	sessionID := cmd.Args().First()
	if err := client.DoJSON(ctx, "DELETE", "/auth/sessions/"+url.PathEscape(sessionID), nil, nil); err != nil {
		return err
	}
	fmt.Fprintf(commandWriter(cmd), "Revoked auth session %s\n", sessionID)
	return nil
}

func runConsole(ctx context.Context, cmd *cli.Command) error {
	client, err := discoverManagedDaemon(ctx)
	if err != nil {
		return err
	}
	consoleURL, err := resolvePairingURL(client.URL(), "/console")
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

func resolvePairingURL(base, pairingPath string) (string, error) {
	parsedBase, err := validatePublicURL(base)
	if err != nil {
		return "", err
	}
	path, err := url.Parse(pairingPath)
	if err != nil {
		return "", fmt.Errorf("parse pairing path: %w", err)
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

func writeAuthSessions(writer io.Writer, sessions []api.AuthSession) {
	if len(sessions) == 0 {
		fmt.Fprintln(writer, "No auth sessions.")
		return
	}
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tCLIENT\tCREATED\tEXPIRES\tSTATUS")
	for _, session := range sessions {
		status := "active"
		if session.RevokedAt != "" {
			status = "revoked"
		} else if expiresAt, err := time.Parse(time.RFC3339Nano, session.ExpiresAt); err == nil && !expiresAt.After(time.Now().UTC()) {
			status = "expired"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", session.ID, session.ClientID, session.CreatedAt, session.ExpiresAt, status)
	}
	_ = table.Flush()
}
