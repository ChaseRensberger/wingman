---
title: "Embed the Daemon"
description: "Run a Wingman daemon inside a Go application."
group: "How-to"
order: 1005
---

# Embed the Daemon

Use the `app` package when a Go program must own a complete Wingman daemon.
The application root owns storage, external plugins, MCP connections, background
workers, and the HTTP adapter.

## Serve on a Listener

Create the listener before the application. A bind failure then occurs before
Wingman opens the database or starts external processes.

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

listener, err := net.Listen("tcp", "127.0.0.1:2323")
if err != nil {
	log.Fatal(err)
}

application, err := app.New(ctx, app.Config{
	DBPath:          "/var/lib/wingman/wingman.db",
	LogFormat:       "json",
	LogLevel:        "info",
	ShutdownTimeout: 30 * time.Second,
})
if err != nil {
	listener.Close()
	log.Fatal(err)
}

if err := application.Serve(ctx, listener); err != nil {
	log.Fatal(err)
}
```

`app.New` completes startup recovery before it returns.

`App.Serve` stops accepting HTTP requests when the context ends and then closes
daemon resources.

## Use the Handler Without a Listener

Use `App.Handler` for exact HTTP behavior in a test or an in-process transport.

```go
application, err := app.New(context.Background(), app.Config{
	Ephemeral:      true,
	DisablePlugins: true,
})
if err != nil {
	t.Fatal(err)
}
defer application.Close(context.Background())

request := httptest.NewRequest(http.MethodGet, "/health", nil)
response := httptest.NewRecorder()
application.Handler().ServeHTTP(response, request)
```

`App.Close` coordinates automatically with `App.Serve`. If another HTTP server
uses `App.Handler`, drain that server before you call `App.Close`.

Use the direct packages under `agent/`, `models/`, and `store/` when a program
does not need daemon HTTP behavior.

## Shutdown Contract

`App.Close` cancels application work, waits for the server to stop, then closes
daemon resources.

If the supplied context ends before server work drains, dependencies remain
open. Call `App.Close` again with a new context to continue shutdown.
