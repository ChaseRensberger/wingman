---
title: "Embed the Daemon"
description: "Run a Wingman daemon inside a Go application."
group: "How-to"
order: 1005
---

# Embed the Daemon

Use the `app` package to run a Wingman server inside a Go application.
It manages storage, plugins, MCP connections, and HTTP requests.

## Serve on a Listener

Create the listener first so a bind failure does not open storage or start external programs.

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

listener, err := net.Listen("tcp", "127.0.0.1:2424")
if err != nil {
	log.Fatal(err)
}

application, err := app.New(ctx, app.Config{
	DBPath:          "/home/alex/.local/share/wingman/wingman.db",
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

When the context ends, `App.Serve` stops accepting HTTP requests. Then it closes daemon resources.

## Use the Handler Without a Listener

Use `App.Handler` to send HTTP requests in a test or through another server:

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

`App.Close` coordinates automatically with `App.Serve`.
If another HTTP server uses `App.Handler`, drain that server before you call `App.Close`.

Use `agent`, `models`, and `store` directly if your application does not need an HTTP server.

## Shutdown Contract

`App.Close` cancels application work. It waits for the server to stop. Then it closes daemon resources.

If the context ends before active work finishes, resources remain open.
Call `App.Close` again with a new context to finish shutdown.
