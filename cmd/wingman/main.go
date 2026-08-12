package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/urfave/cli/v3"

	"github.com/chaserensberger/wingman/app"
	daemonconfig "github.com/chaserensberger/wingman/internal/config"
	"github.com/chaserensberger/wingman/internal/daemonstate"
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

	cmd := newCommand(cfg)

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newCommand(cfg daemonconfig.Config) *cli.Command {
	return &cli.Command{
		Name:  "wingman",
		Usage: "The open-source client-agnostic agent harness",
		Commands: []*cli.Command{
			{
				Name:   "serve",
				Usage:  "Start the HTTP server",
				Flags:  serveCommandFlags(cfg),
				Action: runServe(cfg),
			},
			{
				Name:  "service",
				Usage: "Manage Wingman as a background service",
				Commands: []*cli.Command{
					{
						Name:   "start",
						Usage:  "Install and start Wingman as a background service",
						Flags:  serveFlags(cfg),
						Action: runServiceStart,
					},
					{
						Name:   "stop",
						Usage:  "Stop and remove the Wingman background service",
						Action: runServiceStop,
					},
					{
						Name:   "restart",
						Usage:  "Restart the Wingman background service",
						Action: runServiceRestart,
					},
					{
						Name:   "status",
						Usage:  "Show Wingman's background service status",
						Action: runServiceStatus,
					},
				},
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
			{
				Name:   "pair",
				Usage:  "Show server pairing information",
				Action: runPair(cfg),
			},
			clientsCommand(),
			{
				Name:   "console",
				Usage:  "Open the managed daemon console",
				Action: runConsole,
			},
		},
	}
}

func serveCommandFlags(cfg daemonconfig.Config) []cli.Flag {
	return append(serveFlags(cfg),
		&cli.BoolFlag{Name: "register", Usage: "Publish managed-daemon discovery state", Hidden: true},
		&cli.StringFlag{Name: "state-dir", Usage: "Override the private daemon state directory", Hidden: true},
	)
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
			Name:  "console-dev-url",
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
		stateDir := cmd.String("state-dir")
		if stateDir == "" {
			var err error
			stateDir, err = daemonstate.DefaultDir()
			if err != nil {
				return err
			}
		}
		state := daemonstate.New(stateDir)
		username, password, displayCredentials, err := serverCredentials()
		if err != nil {
			return err
		}
		var lock *daemonstate.Lock
		if cmd.Bool("register") {
			var err error
			lock, err = state.AcquireLock()
			if err != nil {
				return fmt.Errorf("acquire managed daemon ownership: %w", err)
			}
			defer func() { _ = lock.Release() }()
		}
		instanceID := "ins_" + ksuid.New().String()
		sigCtx, stopSig := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stopSig()
		addr := fmt.Sprintf("%s:%d", effective.Server.Host, effective.Server.Port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}
		if cmd.Bool("register") {
			registration := daemonstate.Registration{
				InstanceID: instanceID, Version: version, URL: listenerURL(effective.Server.Host, listener.Addr()), URLs: listenerURLs(effective.Server.Host, listener.Addr()),
				PID: os.Getpid(), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			}
			if err := state.WriteRegistration(registration); err != nil {
				_ = listener.Close()
				return fmt.Errorf("publish daemon registration: %w", err)
			}
			defer func() { _, _ = state.RemoveRegistration(instanceID) }()
		}
		application, err := app.New(sigCtx, app.Config{
			Ephemeral: cmd.Bool("ephemeral"), DBPath: effective.Server.DB,
			ConsoleDevURL: cmd.String("console-dev-url"), LogFormat: effective.Server.LogFormat, LogLevel: effective.Server.LogLevel,
			PluginDirs: effective.Plugins.Dirs, DefaultPluginDir: effective.Plugins.DefaultDir, DisablePlugins: cmd.Bool("no-plugins"),
			MCP: effective.MCP, Providers: effective.Provider,
			Permissions: effective.Permissions, AgentPermissions: effective.AgentPermissions,
			Password: password, Username: username, InstanceID: instanceID, Version: version,
		})
		if err != nil {
			_ = listener.Close()
			return err
		}
		logger := application.Logger()
		slog.SetDefault(logger)
		logger.Info("server starting", "addr", addr)
		if displayCredentials && !cmd.Bool("register") {
			fmt.Printf("server listening on %s\n", listenerURL(effective.Server.Host, listener.Addr()))
			fmt.Printf("server username %s\n", username)
			fmt.Printf("server password %s\n", password)
		}
		if err := application.Serve(sigCtx, listener); err != nil {
			return fmt.Errorf("serve application: %w", err)
		}
		logger.Info("shutdown complete")
		return nil
	}
}

func serverCredentials() (string, string, bool, error) {
	password := os.Getenv("WINGMAN_PASSWORD")
	username := os.Getenv("WINGMAN_USERNAME")
	if password != "" {
		if username == "" {
			username = "wingman"
		}
		return username, password, false, nil
	}
	serviceConfig, err := daemonstate.EnsureServiceConfig()
	if err != nil {
		return "", "", false, fmt.Errorf("load server credentials: %w", err)
	}
	return serviceConfig.Username, serviceConfig.Password, true, nil
}

func listenerURL(configuredHost string, addr net.Addr) string {
	host := configuredHost
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::", "[::]":
		host = "::1"
	}
	port := 0
	if tcp, ok := addr.(*net.TCPAddr); ok {
		port = tcp.Port
	} else if _, value, err := net.SplitHostPort(addr.String()); err == nil {
		port, _ = strconv.Atoi(value)
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

func listenerURLs(configuredHost string, addr net.Addr) []string {
	port := listenerPort(addr)
	if configuredHost != "0.0.0.0" && configuredHost != "::" && configuredHost != "[::]" {
		return []string{listenerURL(configuredHost, addr)}
	}
	family := "ip4"
	if configuredHost == "::" || configuredHost == "[::]" {
		family = "ip6"
	}
	urls := make([]string, 0)
	seen := make(map[string]struct{})
	interfaces, err := net.Interfaces()
	if err != nil {
		return []string{listenerURL(configuredHost, addr)}
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err != nil || ip.IsLoopback() || family == "ip4" && ip.To4() == nil || family == "ip6" && ip.To4() != nil {
				continue
			}
			url := "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(port))
			if _, ok := seen[url]; !ok {
				seen[url] = struct{}{}
				urls = append(urls, url)
			}
		}
	}
	if len(urls) == 0 {
		return []string{listenerURL(configuredHost, addr)}
	}
	return urls
}

func listenerPort(addr net.Addr) int {
	if tcp, ok := addr.(*net.TCPAddr); ok {
		return tcp.Port
	}
	_, value, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0
	}
	port, _ := strconv.Atoi(value)
	return port
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
