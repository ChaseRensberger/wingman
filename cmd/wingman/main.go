package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/chaserensberger/wingman/wingagent/server"
	"github.com/chaserensberger/wingman/wingagent/storage"
	"github.com/chaserensberger/wingman/wingmodels/catalog"
	_ "github.com/chaserensberger/wingman/wingmodels/providers/anthropic"
	_ "github.com/chaserensberger/wingman/wingmodels/providers/ollama"
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
		log.Fatal(err)
	}
}

func runServe(ctx context.Context, cmd *cli.Command) error {
	dbPath := cmd.String("db")
	if dbPath == "" {
		var err error
		dbPath, err = storage.DefaultDBPath()
		if err != nil {
			return fmt.Errorf("failed to get default database path: %w", err)
		}
	}

	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer store.Close()

	srv := server.New(server.Config{
		Store: store,
	})

	host := cmd.String("host")
	port := cmd.Int("port")
	addr := fmt.Sprintf("%s:%d", host, port)

	httpSrv := &http.Server{Addr: addr}

	// Background catalog refresh. Models.dev publishes incremental
	// updates we want to pick up without restarting; failures are
	// silent (the embedded snapshot remains the source of truth).
	catalogStop := make(chan struct{})
	defer close(catalogStop)
	catalog.Default().StartRefresher(time.Hour, catalogStop)

	// SIGINT/SIGTERM → graceful shutdown. Drain has a 30s budget;
	// after that we return with the deadline error and let the
	// process exit (defers still run).
	sigCtx, stopSig := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSig()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(httpSrv) }()

	log.Printf("Database: %s", dbPath)

	select {
	case err := <-serveErr:
		return err
	case <-sigCtx.Done():
		log.Printf("Shutdown signal received; draining (30s budget)...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx, httpSrv); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		// Wait for Serve to return so we don't race the defer-store-Close.
		<-serveErr
		log.Printf("Shutdown complete")
		return nil
	}
}
