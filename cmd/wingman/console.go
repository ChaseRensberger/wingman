package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/internal/daemonclient"
)

type consoleDaemonClient interface {
	URL() string
	DoJSON(context.Context, string, string, any, any) error
}

func consoleTargetURL(ctx context.Context, client consoleDaemonClient, directories []string) (string, error) {
	if len(directories) == 0 {
		return resolveURL(client.URL(), "/console")
	}
	dir, err := session.ResolveWorkDir(directories[0])
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", errors.New("working directory is required")
	}
	var workspaces []api.Workspace
	if err := client.DoJSON(ctx, http.MethodGet, "/workspaces", nil, &workspaces); err != nil {
		return "", err
	}
	workspace, found := consoleWorkspaceForDirectory(workspaces, dir)
	for attempt := 0; !found && attempt < 2; attempt++ {
		name := filepath.Base(dir)
		for _, existing := range workspaces {
			if strings.EqualFold(existing.Name, name) {
				name = dir
				break
			}
		}
		err := client.DoJSON(ctx, http.MethodPost, "/workspaces", api.CreateWorkspaceRequest{Name: name, Path: dir}, &workspace)
		if err == nil {
			break
		}
		var apiErr *daemonclient.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
			return "", err
		}
		// Another invocation can claim this directory or name after the initial list.
		if listErr := client.DoJSON(ctx, http.MethodGet, "/workspaces", nil, &workspaces); listErr != nil {
			return "", listErr
		}
		workspace, found = consoleWorkspaceForDirectory(workspaces, dir)
		if !found && (name == dir || attempt == 1) {
			return "", fmt.Errorf("create workspace: %w", err)
		}
	}
	return resolveURL(client.URL(), "/console/sessions/new?"+url.Values{"workspace": {workspace.ID}}.Encode())
}

func consoleWorkspaceForDirectory(workspaces []api.Workspace, dir string) (api.Workspace, bool) {
	for _, workspace := range workspaces {
		if workspace.Path != "" && filepath.Clean(workspace.Path) == dir {
			return workspace, true
		}
	}
	return api.Workspace{}, false
}
