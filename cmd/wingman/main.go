package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/chaserensberger/wingman/internal/observability"
	_ "github.com/chaserensberger/wingman/models/providers/anthropic"
	_ "github.com/chaserensberger/wingman/models/providers/openai"
	_ "github.com/chaserensberger/wingman/models/providers/opencode"
	"github.com/chaserensberger/wingman/server"
	"github.com/chaserensberger/wingman/store"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd := &cli.Command{
		Name:  "wingman",
		Usage: "AI agent framework",
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "Start the HTTP server",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "log-format",
						Value: "json",
						Usage: "Log format: json or text",
					},
					&cli.StringFlag{
						Name:  "log-level",
						Value: "info",
						Usage: "Log level: debug, info, warn, or error",
					},
					&cli.IntFlag{
						Name:  "port",
						Value: 2323,
						Usage: "Port to listen on",
					},
					&cli.StringFlag{
						Name:  "host",
						Value: "127.0.0.1",
						Usage: "Host to bind to",
					},
					&cli.StringFlag{
						Name:  "db",
						Usage: "Database path (default: ~/.local/share/wingman/wingman.db)",
					},
					&cli.StringFlag{
						Name:  "ui-dev",
						Usage: "Proxy /web to a Vite dev server URL",
					},
					&cli.BoolFlag{
						Name:  "ephemeral",
						Usage: "Run in ephemeral mode without persistence",
					},
				},
				Action: runServe,
			},
			{
				Name:  "version",
				Usage: "Print version information",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					fmt.Printf("wingman %s (commit: %s, built: %s)\n", version, commit, date)
					return nil
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServe(ctx context.Context, cmd *cli.Command) error {
	logger, err := observability.ConfigureDefault(cmd.String("log-format"), cmd.String("log-level"))
	if err != nil {
		return err
	}

	var st store.Store
	if cmd.Bool("ephemeral") {
		logger.Info("persistence disabled", "mode", "ephemeral")
	} else {
		dbPath := cmd.String("db")
		if dbPath == "" {
			dbPath, err = store.DefaultDBPath()
			if err != nil {
				return fmt.Errorf("failed to get default database path: %w", err)
			}
		}
		sqliteStore, err := store.NewSQLiteStore(dbPath)
		if err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}
		defer sqliteStore.Close()
		st = sqliteStore
		logger.Info("storage initialized", "db_path", dbPath)
	}

	srv := server.New(server.Config{
		Store:     st,
		WebDevURL: cmd.String("ui-dev"),
		Logger:    logger,
	})

	host := cmd.String("host")
	port := cmd.Int("port")
	addr := fmt.Sprintf("%s:%d", host, port)

	httpSrv := &http.Server{Addr: addr}

	// SIGINT/SIGTERM → graceful shutdown. Drain has a 30s budget;
	// after that we return with the deadline error and let the
	// process exit (defers still run).
	sigCtx, stopSig := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSig()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(httpSrv) }()

	select {
	case err := <-serveErr:
		return err
	case <-sigCtx.Done():
		logger.Info("shutdown signal received", "budget", "30s")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx, httpSrv); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		// Wait for Serve to return so we don't race the defer-store-Close.
		<-serveErr
		logger.Info("shutdown complete")
		return nil
	}
}
