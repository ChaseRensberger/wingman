---
title: "Embed the Daemon"
description: "Run a Wingman HTTP server inside a Go application."
---

# Embed the Daemon

The `app` package runs a Wingman HTTP server inside your Go application.
It owns storage, plugins, MCP connections, and server shutdown.
Use `agent`, `models`, and `store` directly if you do not need HTTP.

## Start a server

Set `WINGMAN_PASSWORD` before running this example. Unlike the CLI, `app.New` does not generate credentials or load `wingman.json`.
An empty `Config.Password` disables HTTP authentication.

```go
package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/chaserensberger/wingman/app"
)

func main() {
	password := os.Getenv("WINGMAN_PASSWORD")
	if password == "" {
		log.Fatal("WINGMAN_PASSWORD is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", "127.0.0.1:2424")
	if err != nil {
		log.Fatal(err)
	}
	application, err := app.New(ctx, app.Config{
		Username: "wingman",
		Password: password,
		DBPath:   "./wingman.db",
	})
	if err != nil {
		listener.Close()
		log.Fatal(err)
	}
	if err := application.Serve(ctx, listener); err != nil {
		log.Fatal(err)
	}
}
```

Opening the listener first avoids starting storage and external programs when the port is unavailable.
`app.New` completes startup recovery before it returns.
When the context ends, `Serve` stops HTTP requests and closes application resources.

## Use an existing HTTP server

`App.Handler()` returns the HTTP handler. If another server uses it, stop that server's requests before calling `App.Close(ctx)`.
`App.Close` cancels work and waits for it to finish.
If its context ends first, resources remain open. Call it again with a new context to finish shutdown.
