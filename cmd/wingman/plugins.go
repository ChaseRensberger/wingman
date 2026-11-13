package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/pluginhub"
	"github.com/urfave/cli/v3"
)

func pluginsCommand() *cli.Command {
	argument := func(name string) []cli.Argument { return []cli.Argument{&cli.StringArgs{Name: name, Min: 1, Max: 1}} }
	manage := func(operation string) cli.ActionFunc {
		return func(ctx context.Context, cmd *cli.Command) error {
			return pluginAPI(ctx, cmd, http.MethodPost, "/rpc-plugins/manage", map[string]string{"operation": operation, "source": cmd.StringArgs("source")[0]})
		}
	}
	trust := func(trusted bool) cli.ActionFunc {
		return func(ctx context.Context, cmd *cli.Command) error {
			directory, err := filepath.Abs(cmd.StringArgs("directory")[0])
			if err != nil {
				return err
			}
			return pluginAPI(ctx, cmd, http.MethodPost, "/rpc-plugins/trust", map[string]any{"directory": directory, "trusted": trusted})
		}
	}
	return &cli.Command{Name: "plugin", Usage: "Manage native Wingman plugins", Commands: []*cli.Command{
		{Name: "call", Usage: "Call a declared plugin operation", Arguments: []cli.Argument{&cli.StringArgs{Name: "operation", Min: 2, Max: 2}}, Flags: []cli.Flag{&cli.StringFlag{Name: "data", Value: "{}", Usage: "JSON input"}, &cli.StringFlag{Name: "directory", Usage: "Project directory on the server"}}, Action: func(ctx context.Context, cmd *cli.Command) error {
			input := json.RawMessage(cmd.String("data"))
			if !json.Valid(input) {
				return fmt.Errorf("data must be JSON")
			}
			args := cmd.StringArgs("operation")
			return pluginAPI(ctx, cmd, http.MethodPost, "/rpc-plugins/call", map[string]any{"plugin": args[0], "method": args[1], "input": input, "working_directory": cmd.String("directory")})
		}},
		{Name: "install", Aliases: []string{"add"}, Usage: "Download and activate a plugin from the configured catalog", Arguments: argument("source"), Action: manage("install")},
		{Name: "update", Usage: "Update an installed plugin from its catalog", Arguments: argument("source"), Action: manage("update")},
		{Name: "remove", Usage: "Remove an installed plugin", Arguments: argument("source"), Action: manage("remove")},
		{Name: "list", Usage: "List installed plugin packages", Action: func(ctx context.Context, cmd *cli.Command) error {
			return pluginAPI(ctx, cmd, http.MethodGet, "/rpc-plugins/packages", nil)
		}},
		{Name: "inspect", Usage: "Inspect active plugin capabilities and diagnostics", Action: func(ctx context.Context, cmd *cli.Command) error {
			return pluginAPI(ctx, cmd, http.MethodGet, "/rpc-plugins", nil)
		}},
		{Name: "search", Usage: "Search the configured plugin catalog", Arguments: argument("query"), Action: func(ctx context.Context, cmd *cli.Command) error {
			return pluginAPI(ctx, cmd, http.MethodGet, "/rpc-plugins/catalog?query="+url.QueryEscape(cmd.StringArgs("query")[0]), nil)
		}},
		{Name: "trust", Usage: "Allow a project's plugins to execute with the server's permissions", Arguments: argument("directory"), Action: trust(true)},
		{Name: "untrust", Usage: "Disable project plugins for subsequent runs", Arguments: argument("directory"), Action: trust(false)},
		{Name: "validate", Usage: "Start and validate a local plugin directory", Arguments: argument("directory"), Action: func(ctx context.Context, cmd *cli.Command) error {
			dir, err := filepath.Abs(cmd.StringArgs("directory")[0])
			if err != nil {
				return err
			}
			manager, err := pluginhost.New(ctx, []string{dir})
			if err != nil {
				return err
			}
			defer manager.Close()
			status, _ := manager.Status()
			if len(status) == 0 {
				return fmt.Errorf("no plugin manifests found")
			}
			return json.NewEncoder(commandWriter(cmd)).Encode(status)
		}},
		{Name: "validate-catalog", Usage: "Validate a local release catalog without executing plugins", Arguments: argument("file"), Action: func(ctx context.Context, cmd *cli.Command) error {
			data, err := os.ReadFile(cmd.StringArgs("file")[0])
			if err != nil {
				return err
			}
			var catalog pluginhub.Catalog
			if err := json.Unmarshal(data, &catalog); err != nil {
				return err
			}
			if err := catalog.Validate(); err != nil {
				return err
			}
			_, err = fmt.Fprintln(commandWriter(cmd), "Plugin catalog is valid")
			return err
		}},
	}}
}

func pluginAPI(ctx context.Context, cmd *cli.Command, method, path string, input any) error {
	client, err := discoverManagedDaemon(ctx)
	if err != nil {
		return err
	}
	return pluginAPIWithClient(ctx, commandWriter(cmd), client, method, path, input)
}

func pluginAPIWithClient(ctx context.Context, output io.Writer, client rawDaemonClient, method, path string, input any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	response, err := client.Do(ctx, method, path, body, http.Header{"Content-Type": []string{"application/json"}})
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if _, err := io.Copy(output, response.Body); err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &apiStatusError{status: response.Status}
	}
	return nil
}
