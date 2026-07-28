package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/chaserensberger/wingman/internal/observability"
	wingmcp "github.com/chaserensberger/wingman/mcp"
	provider "github.com/chaserensberger/wingman/models/providers"
	_ "github.com/chaserensberger/wingman/models/providers/anthropic"
	_ "github.com/chaserensberger/wingman/models/providers/deepseek"
	_ "github.com/chaserensberger/wingman/models/providers/google"
	_ "github.com/chaserensberger/wingman/models/providers/openai"
	_ "github.com/chaserensberger/wingman/models/providers/openaicompat"
	_ "github.com/chaserensberger/wingman/models/providers/opencode"
	_ "github.com/chaserensberger/wingman/models/providers/opencodego"
	_ "github.com/chaserensberger/wingman/models/providers/openrouter"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/server"
	"github.com/chaserensberger/wingman/store"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type fileConfig struct {
	Server struct {
		Host      string `json:"host"`
		Port      int    `json:"port"`
		DB        string `json:"db"`
		LogLevel  string `json:"log_level"`
		LogFormat string `json:"log_format"`
	} `json:"server"`
	Plugins struct {
		Dirs []string `json:"dirs"`
	} `json:"plugins"`
	Models struct {
		Default string `json:"default"`
	} `json:"models"`
	Permissions      permission.Ruleset                 `json:"permissions"`
	AgentPermissions map[string]permission.Ruleset      `json:"agent_permissions"`
	Provider         map[string]provider.ProviderConfig `json:"provider"`
	MCP              map[string]wingmcp.ServerConfig    `json:"mcp"`
}

func loadConfig() (fileConfig, error) {
	var cfg fileConfig
	configDir, err := configDir()
	if err != nil {
		return cfg, fmt.Errorf("resolve config directory: %w", err)
	}
	path := filepath.Join(configDir, "wingman", "wingman.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func configDir() (string, error) {
	if os.Geteuid() != 0 || os.Getenv("SUDO_USER") == "" {
		if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
			return dir, nil
		}
	}
	home, err := effectiveHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

func (c fileConfig) host() string {
	if c.Server.Host != "" {
		return c.Server.Host
	}
	return "127.0.0.1"
}

func (c fileConfig) port() int {
	if c.Server.Port != 0 {
		return c.Server.Port
	}
	return 2323
}

func (c fileConfig) db() string {
	return expandHome(c.Server.DB)
}

func (c fileConfig) logLevel() string {
	if c.Server.LogLevel != "" {
		return c.Server.LogLevel
	}
	return "info"
}

func (c fileConfig) logFormat() string {
	if c.Server.LogFormat != "" {
		return c.Server.LogFormat
	}
	return "json"
}

func (c fileConfig) pluginDirs() []string {
	if len(c.Plugins.Dirs) == 0 {
		return nil
	}
	dirs := make([]string, len(c.Plugins.Dirs))
	for i, dir := range c.Plugins.Dirs {
		dirs[i] = expandHome(dir)
	}
	return dirs
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := effectiveHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := effectiveHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func effectiveHomeDir() (string, error) {
	if os.Geteuid() == 0 && os.Getenv("SUDO_USER") != "" {
		u, err := user.Lookup(os.Getenv("SUDO_USER"))
		if err != nil {
			return "", err
		}
		return u.HomeDir, nil
	}
	return os.UserHomeDir()
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

func serveFlags(cfg fileConfig) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "log-format",
			Value: cfg.logFormat(),
			Usage: "Log format: json or text",
		},
		&cli.StringFlag{
			Name:  "log-level",
			Value: cfg.logLevel(),
			Usage: "Log level: debug, info, warn, or error",
		},
		&cli.IntFlag{
			Name:  "port",
			Value: cfg.port(),
			Usage: "Port to listen on",
		},
		&cli.StringFlag{
			Name:  "host",
			Value: cfg.host(),
			Usage: "Host to bind to",
		},
		&cli.StringFlag{
			Name:  "db",
			Value: cfg.db(),
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
			Value: cfg.pluginDirs(),
			Usage: "Additional global plugin directory (can be repeated)",
		},
		&cli.BoolFlag{
			Name:  "no-plugins",
			Usage: "Disable out-of-process plugin loading",
		},
	}
}

func runServe(cfg fileConfig) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		logs := observability.NewLogBuffer(500)
		logger, err := observability.ConfigureDefaultWithBuffer(cmd.String("log-format"), cmd.String("log-level"), logs)
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

		var plugins *pluginhost.Manager
		if !cmd.Bool("no-plugins") {
			dirs := []string{}
			defaultPluginDir, err := pluginhost.DefaultGlobalDir()
			if err != nil {
				return fmt.Errorf("failed to get default plugin directory: %w", err)
			}
			dirs = append(dirs, defaultPluginDir)
			dirs = append(dirs, cmd.StringSlice("plugin-dir")...)
			plugins, err = pluginhost.New(ctx, dirs)
			if err != nil {
				return fmt.Errorf("failed to initialize plugins: %w", err)
			}
			defer plugins.Close()
			logger.Info("plugins initialized", "dirs", dirs)
		}

		mcpManager := wingmcp.New(ctx, wingmcp.Config{Servers: cfg.MCP})
		defer mcpManager.Close()

		srv := server.New(server.Config{
			Store:            st,
			WebDevURL:        cmd.String("ui-dev"),
			Logger:           logger,
			Logs:             logs,
			Plugins:          plugins,
			MCP:              mcpManager,
			Providers:        cfg.Provider,
			Permissions:      cfg.Permissions,
			AgentPermissions: cfg.AgentPermissions,
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
