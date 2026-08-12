package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/mdp/qrterminal/v3"
	"github.com/urfave/cli/v3"

	daemonconfig "github.com/chaserensberger/wingman/internal/config"
	"github.com/chaserensberger/wingman/internal/daemonclient"
	"github.com/chaserensberger/wingman/internal/daemonstate"
)

type pairingInfo struct {
	URLs     []string `json:"urls"`
	Username string   `json:"username"`
	Password string   `json:"password"`
}

func runPair(cfg daemonconfig.Config) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		stateDir, err := managedStateDir()
		if err != nil {
			return err
		}
		state := daemonstate.New(stateDir)
		if result := daemonclient.Inspect(ctx, state, version); result.Status == daemonclient.StatusMissing {
			if err := runServiceStartWithOptions(ctx, defaultServiceOptions(cfg)); err != nil {
				return err
			}
		}
		client, err := daemonclient.New(ctx, state, version)
		if err != nil {
			return err
		}
		credentials, err := daemonstate.ReadServiceConfig()
		if err != nil {
			return fmt.Errorf("read managed daemon credentials: %w", err)
		}
		return writePairingInfo(commandWriter(cmd), pairingInfo{
			URLs:     client.URLs(),
			Username: credentials.Username,
			Password: credentials.Password,
		})
	}
}

func defaultServiceOptions(cfg daemonconfig.Config) serviceOptions {
	return serviceOptions{
		Host: cfg.Server.Host, Port: cfg.Server.Port, DB: cfg.Server.DB,
		LogFormat: cfg.Server.LogFormat, LogLevel: cfg.Server.LogLevel,
		PluginDirs: append([]string(nil), cfg.Plugins.Dirs...),
	}
}

func writePairingInfo(writer io.Writer, info pairingInfo) error {
	if len(info.URLs) == 0 {
		return fmt.Errorf("pairing information has no server URL")
	}
	payload, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("encode pairing information: %w", err)
	}
	fmt.Fprintf(writer, "\n  URLs      %s\n", info.URLs[0])
	for _, raw := range info.URLs[1:] {
		fmt.Fprintf(writer, "            %s\n", raw)
	}
	fmt.Fprintf(writer, "  Username  %s\n  Password  %s\n\n  Scan to pair\n\n", info.Username, info.Password)
	qrterminal.GenerateWithConfig(string(payload), qrterminal.Config{
		Level:      qrterminal.L,
		Writer:     writer,
		HalfBlocks: true,
		QuietZone:  2,
	})
	parsed, err := url.Parse(info.URLs[0])
	if err == nil && isLoopbackHost(parsed.Hostname()) {
		fmt.Fprintln(writer, "\n  Run `wingman service start --host 0.0.0.0` to access the service remotely.")
	}
	return nil
}
