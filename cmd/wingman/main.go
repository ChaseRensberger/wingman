package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"os/user"
	"syscall"

	"github.com/urfave/cli/v3"

	"github.com/chaserensberger/wingman/app"
	daemonconfig "github.com/chaserensberger/wingman/internal/config"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func loadConfig() (daemonconfig.Config, error) {
	path, err := daemonconfig.DefaultPath()
	if err != nil {
		return daemonconfig.Config{}, err
	}
	cfg, err := daemonconfig.Load(path)
	if err != nil {
		return daemonconfig.Config{}, err
	}
	home, err := daemonconfig.HomeDir()
	if err != nil {
		return daemonconfig.Config{}, err
	}
	return cfg.Normalize(home)
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cmd := &cli.Command{
		Name:  "wingman",
		Usage: "The open-source client-agnostic agent harness",
		Commands: []*cli.Command{
			{
				Name:   "serve",
				Usage:  "Start the HTTP server",
				Flags:  serveFlags(cfg),
				Action: runServe(cfg),
			},
			{
				Name:   "up",
				Usage:  "Install and start Wingman as a background service",
				Flags:  serveFlags(cfg),
				Action: runUp,
			},
			{
				Name:   "down",
				Usage:  "Stop and remove the Wingman background service",
				Action: runDown,
			},
			{
				Name:   "restart",
				Usage:  "Restart the Wingman background service",
				Action: runRestart,
			},
			{
				Name:   "status",
				Usage:  "Show Wingman's background service status",
				Action: runStatus,
			},
			{
				Name:  "version",
				Usage: "Print version information",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					fmt.Printf("wingman %s (commit: %s, built: %s)\n", version, commit, date)
					return nil
				},
			},
			{
				Name:   "update",
				Usage:  "Install a verified Wingman release",
				Flags:  updateFlags(),
				Action: runUpdate,
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func serveFlags(cfg daemonconfig.Config) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "log-format",
			Value: cfg.Server.LogFormat,
			Usage: "Log format: json or text",
		},
		&cli.StringFlag{
			Name:  "log-level",
			Value: cfg.Server.LogLevel,
			Usage: "Log level: debug, info, warn, or error",
		},
		&cli.IntFlag{
			Name:  "port",
			Value: cfg.Server.Port,
			Usage: "Port to listen on",
		},
		&cli.StringFlag{
			Name:  "host",
			Value: cfg.Server.Host,
			Usage: "Host to bind to",
		},
		&cli.StringFlag{
			Name:  "db",
			Value: cfg.Server.DB,
			Usage: "Database path (default: ~/.local/share/wingman/wingman.db)",
		},
		&cli.StringFlag{
			Name:  "ui-dev",
			Usage: "Proxy /console to a Vite dev server URL",
		},
		&cli.BoolFlag{
			Name:  "ephemeral",
			Usage: "Run in ephemeral mode without persistence",
		},
		&cli.StringSliceFlag{
			Name:  "plugin-dir",
			Value: cfg.Plugins.Dirs,
			Usage: "Additional global plugin directory (can be repeated)",
		},
		&cli.BoolFlag{
			Name:  "no-plugins",
			Usage: "Disable out-of-process plugin loading",
		},
	}
}

func runServe(cfg daemonconfig.Config) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		effective := cfg
		effective.Server.Host = cmd.String("host")
		effective.Server.Port = cmd.Int("port")
		effective.Server.DB = cmd.String("db")
		effective.Server.LogLevel = cmd.String("log-level")
		effective.Server.LogFormat = cmd.String("log-format")
		effective.Plugins.Dirs = append([]string(nil), cmd.StringSlice("plugin-dir")...)
		if err := effective.Validate(); err != nil {
			return fmt.Errorf("validate effective config: %w", err)
		}
		sigCtx, stopSig := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stopSig()
		addr := fmt.Sprintf("%s:%d", effective.Server.Host, effective.Server.Port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}
		application, err := app.New(sigCtx, app.Config{
			Ephemeral: cmd.Bool("ephemeral"), DBPath: effective.Server.DB,
			WebDevURL: cmd.String("ui-dev"), LogFormat: effective.Server.LogFormat, LogLevel: effective.Server.LogLevel,
			PluginDirs: effective.Plugins.Dirs, DefaultPluginDir: effective.Plugins.DefaultDir, DisablePlugins: cmd.Bool("no-plugins"),
			MCP: effective.MCP, Providers: effective.Provider,
			Permissions: effective.Permissions, AgentPermissions: effective.AgentPermissions,
		})
		if err != nil {
			_ = listener.Close()
			return err
		}
		logger := application.Logger()
		slog.SetDefault(logger)
		logger.Info("server starting", "addr", addr)
		if err := application.Serve(sigCtx, listener); err != nil {
			return fmt.Errorf("serve application: %w", err)
		}
		logger.Info("shutdown complete")
		return nil
	}
}

func serviceAccount() (string, string, error) {
	name := os.Getenv("SUDO_USER")
	if name == "" {
		current, err := user.Current()
		if err != nil {
			return "", "", fmt.Errorf("resolve current user: %w", err)
		}
		return current.Username, current.HomeDir, nil
	}

	u, err := user.Lookup(name)
	if err != nil {
		return "", "", fmt.Errorf("resolve sudo user %q: %w", name, err)
	}
	return u.Username, u.HomeDir, nil
}
