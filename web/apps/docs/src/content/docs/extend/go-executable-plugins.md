---
title: "Go executable plugins"
description: "Write and distribute native Go plugins without rebuilding Wingman."
---

# Go executable plugins

A native plugin is a separate Go program that uses `github.com/chaserensberger/wingman/plugin`.
Wingman starts the executable and calls its registered handlers. Write diagnostics to standard error. Standard output belongs to the SDK.

## Register handlers

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/plugin"
)

func main() {
	err := plugin.Serve(context.Background(), plugin.Definition{
		ID:      "acme/context",
		Name:    "Acme context",
		Version: "v1.0.0",
		Setup: func(ctx context.Context, host *plugin.Host, config map[string]any) (*plugin.Extension, error) {
			return &plugin.Extension{
				Hooks: run.Hooks{
					TransformSystem: func(ctx context.Context, info run.TransformSystemInfo) (string, error) {
						return info.System + "\n\nKeep answers concise.", nil
					},
				},
			}, nil
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

Use a publisher-qualified ID and an exact semantic version with a `v` prefix.
The package ID, executable ID, and catalog release version must agree.

`Extension` can register tools, session actions, model providers, custom RPC methods, and lifecycle observers.
Its hooks support initial history, system instructions, context, history, tool definitions, model parameters, tool interception, completion validation, and run completion.
Hooks compose in plugin-ID order after bundled session plugins. Hook calls have a 30-second deadline and receive cancellation.

Use [Executable plugin runtime](/extend/plugin-runtime) for reversible transforms, prompt preparation, native HTTP hooks, sign-in methods, and public event subscriptions.

Use `plugin.NewMethod` to register typed Go inputs and outputs with JSON Schema declarations.
Other plugins call the operation through `host.CallPlugin`. API clients call `POST /rpc-plugins/call`, or use:

```sh
wingman plugin call acme/context operation --data '{}'
```

`GET /actions` lists global session actions. Supply the `working_directory` query parameter to include a trusted project's actions.

## Use host services

`host.Sessions()` exposes typed session creation, prompts, inspection, and cancellation. `host.Request` calls the supported JSON APIs for agents, sessions, Workspaces, model discovery, and plugin operations.
Wingman supplies the invoking client's identity. Plugins do not receive the server's HTTP credentials.
Session prompts return an admitted run ID. Inspect that run to obtain its eventual result.

`StorageGet`, `StorageSet`, and `StorageRemove` store JSON values under the plugin, client, and directory.
Directory scopes use canonical paths, so a project and its symlink aliases share storage.
`CredentialGet`, `CredentialSet`, and `CredentialRemove` store plugin-owned credentials under the plugin and client, shared across directories.
An authentication operation can save its credential and return a sign-in URL or a status without returning the credential to the caller.
Model provider handlers receive the configured provider credential. If none is configured, the SDK looks for the plugin-owned credential.
`protocol.ProviderCall.Options` supplies provider routing options to preparation and generation. Provider handlers apply `BaseURL`, `Query`, and authentication-header settings to their requests.
Setting the provider's `options.auth` to `false` disables both credential sources.
Slow stream consumers fail their request if its delivery buffer fills. Other calls to the plugin remain available.
Wingman delivers accepted stream parts before a terminal provider error, unless the caller cancels the request.
Return a `models.ProviderError` to preserve failure classification, retry timing, and provider request IDs.
Emit the first stream part only after the upstream stream opens. Failures before that part can use the normal startup retry policy. Failures after that part do not retry.

These callbacks require an active invocation. For background work, call `host.Background(ctx)` from a handler and use the returned context.
Cancelling a host call cancels that operation, not its parent invocation or sibling calls.
The SDK cancels that context when the extension shuts down. Return an idempotent `Close` handler that releases resources and waits for owned workers.
`Close` receives a separate five-second cleanup context. Wingman waits for the shutdown response and reports cleanup errors.
Background grants retain client identity, not a completed run's model client, event sink, or captured plugin generation.
Directory-scoped processes can unload when their execution scope becomes idle. Use a directoryless custom operation for a service that must remain active while the daemon runs.

Handlers can run concurrently. Protect shared state and use the supplied context for network calls and subprocesses.
`Setup` can run during validation and replacement. Do not perform irreversible work there.
Observers are bounded and can drop events when slow. Do not use them as a reliable event archive.

## Test a local executable

Build your program, then create a `wingman-plugin.json` in a local plugin directory:

```json
{
  "id": "acme/context",
  "command": ["/absolute/path/to/context-plugin"]
}
```

Validate the directory before installation:

```sh
wingman plugin validate /absolute/path/to/plugin-directory
```

Validation executes the program. Use only code that you trust.

## Publish a release

Build an executable for each supported operating system and CPU. For a pure-Go plugin, a Linux build can use:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o context-linux-amd64 .
sha256sum context-linux-amd64
```

Embed required assets with Go's `embed` package. Release artifacts are executable files, not archives.
Plugins that use native libraries must supply and document their runtime requirements.

Upload the executable to an immutable HTTPS release location. Add the package to the catalog maintained by your hub:

```json
{
  "version": 1,
  "packages": [
    {
      "id": "acme/context",
      "name": "Acme context",
      "description": "Project-specific instructions",
      "repository": "https://github.com/acme/context",
      "releases": [
        {
          "version": "v1.0.0",
          "protocol": 2,
          "artifacts": {
            "linux/amd64": {
              "url": "https://github.com/acme/context/releases/download/v1.0.0/context-linux-amd64",
              "sha256": "REPLACE_WITH_THE_64_CHARACTER_SHA256"
            }
          }
        }
      ]
    }
  ]
}
```

Replace the checksum, then validate the catalog:

```sh
wingman plugin validate-catalog catalog.json
```

`minimum_wingman` can require a minimum Wingman release, such as `v1.2.0`.
The installer selects a compatible stable release for the server platform. It rejects changed artifacts for an already installed release version.
Catalog maintainers control publisher namespaces and review submissions. The catalog format does not provide publisher accounts or automated security review.
