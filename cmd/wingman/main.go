package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v3"

	"wingman/internal/server"
	"wingman/internal/storage"
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
						Value: 8080,
						Usage: "Port to listen on",
					},
					&cli.StringFlag{
						Name:  "host",
						Value: "127.0.0.1",
						Usage: "Host to bind to",
					},
					&cli.StringFlag{
						Name:  "data-dir",
						Usage: "Data directory (default: ~/.local/share/wingman)",
					},
				},
				Action: runServe,
			},
			{
				Name:  "version",
				Usage: "Print version information",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					fmt.Println("wingman v0.1.0")
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
	dataDir := cmd.String("data-dir")
	if dataDir == "" {
		var err error
		dataDir, err = storage.DefaultBasePath()
		if err != nil {
			return fmt.Errorf("failed to get default data directory: %w", err)
		}
	}

	store, err := storage.NewJSONStore(dataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	srv := server.New(server.Config{
		Store: store,
	})

	host := cmd.String("host")
	port := cmd.Int("port")
	addr := fmt.Sprintf("%s:%d", host, port)

	log.Printf("Data directory: %s", dataDir)
	return srv.ListenAndServe(addr)
}
