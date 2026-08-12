package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/chaserensberger/wingman/internal/daemonclient"
	"github.com/chaserensberger/wingman/internal/daemonstate"
)

const (
	systemdServiceName = "wingman.service"
	launchdLabel       = "actor.wingman"
)

func runServiceStart(ctx context.Context, cmd *cli.Command) error {
	switch runtime.GOOS {
	case "linux":
		return runLinuxStart(ctx, cmd)
	case "darwin":
		return runDarwinStart(ctx, cmd)
	default:
		return fmt.Errorf("wingman service start supports Linux/systemd and macOS/launchd only")
	}
}

func runServiceStop(ctx context.Context, cmd *cli.Command) error {
	switch runtime.GOOS {
	case "linux":
		return runLinuxStop(ctx)
	case "darwin":
		return runDarwinStop(ctx)
	default:
		return fmt.Errorf("wingman service stop supports Linux/systemd and macOS/launchd only")
	}
}

func runServiceRestart(ctx context.Context, cmd *cli.Command) error {
	stateDir, err := managedStateDir()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "linux":
		if os.Geteuid() == 0 {
			return fmt.Errorf("run wingman service restart as the logged-in user, not root")
		}
		if err := runSystemctl(ctx, "restart", systemdServiceName); err != nil {
			return err
		}
	case "darwin":
		if err := runLaunchctl(ctx, "kickstart", "-k", launchdTarget()); err != nil {
			return err
		}
	default:
		return fmt.Errorf("wingman service restart supports Linux/systemd and macOS/launchd only")
	}
	return waitForServiceReady(ctx, stateDir)
}

func runServiceStatus(ctx context.Context, cmd *cli.Command) error {
	stateDir, err := managedStateDir()
	if err == nil {
		printDaemonStatus(ctx, daemonstate.New(stateDir))
	}
	switch runtime.GOOS {
	case "linux":
		if os.Geteuid() == 0 {
			return fmt.Errorf("run wingman service status as the logged-in user, not root")
		}
		if _, err := exec.LookPath("systemctl"); err != nil {
			return fmt.Errorf("systemctl not found: %w", err)
		}
		return runSystemctlAttached(ctx, "status", systemdServiceName)
	case "darwin":
		return runLaunchctlAttached(ctx, "print", launchdTarget())
	default:
		return fmt.Errorf("wingman service status supports Linux/systemd and macOS/launchd only")
	}
}

func runLinuxStart(ctx context.Context, cmd *cli.Command) error {
	if os.Geteuid() == 0 {
		return fmt.Errorf("run wingman service start as the logged-in user, not root")
	}
	exe, err := executablePath()
	if err != nil {
		return err
	}
	stateDir, err := daemonstate.DefaultDir()
	if err != nil {
		return err
	}
	_, err = daemonstate.EnsureServiceConfig()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	path := systemdUnitPath(home, configHome)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create user systemd directory: %w", err)
	}
	configPath, err := daemonstate.ServiceConfigPath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(systemdUnit(home, configHome, configPath, serveArgs(exe, cmd, stateDir))), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "enable", systemdServiceName); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "restart", systemdServiceName); err != nil {
		return err
	}
	if err := waitForServiceReady(ctx, stateDir); err != nil {
		return err
	}
	fmt.Println("Wingman service installed and started")
	fmt.Println("Run 'wingman service status' to inspect it")
	return nil
}

func runLinuxStop(ctx context.Context) error {
	if os.Geteuid() == 0 {
		return fmt.Errorf("run wingman service stop as the logged-in user, not root")
	}
	if err := runSystemctl(ctx, "disable", "--now", systemdServiceName); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	if err := os.Remove(systemdUnitPath(home, os.Getenv("XDG_CONFIG_HOME"))); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove user systemd unit: %w", err)
	}
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	fmt.Println("Wingman service stopped and removed")
	return nil
}

func runDarwinStart(ctx context.Context, cmd *cli.Command) error {
	if os.Geteuid() == 0 {
		return fmt.Errorf("run wingman service start as the logged-in user, not root")
	}
	exe, err := executablePath()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	path := launchdPlistPath(home)
	stateDir, err := daemonstate.DefaultDir()
	if err != nil {
		return err
	}
	_, err = daemonstate.EnsureServiceConfig()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create LaunchAgents directory: %w", err)
	}
	configPath, err := daemonstate.ServiceConfigPath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(launchdPlist(home, os.Getenv("XDG_CONFIG_HOME"), configPath, serveArgs(exe, cmd, stateDir))), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	_ = runLaunchctl(ctx, "bootout", launchdTarget())
	if err := runLaunchctl(ctx, "bootstrap", launchdDomain(), path); err != nil {
		return err
	}
	if err := runLaunchctl(ctx, "kickstart", "-k", launchdTarget()); err != nil {
		return err
	}
	if err := waitForServiceReady(ctx, stateDir); err != nil {
		return err
	}
	fmt.Println("Wingman service installed and started")
	fmt.Println("Run 'wingman service status' to inspect it")
	return nil
}

func runDarwinStop(ctx context.Context) error {
	if os.Geteuid() == 0 {
		return fmt.Errorf("run wingman service stop as the logged-in user, not root")
	}
	if err := runLaunchctl(ctx, "bootout", launchdTarget()); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	if err := os.Remove(launchdPlistPath(home)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove LaunchAgent: %w", err)
	}
	fmt.Println("Wingman service stopped and removed")
	return nil
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve wingman binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

func serveArgs(exe string, cmd *cli.Command, stateDir string) []string {
	args := []string{exe, "serve", "--register", "--state-dir", stateDir, "--host", cmd.String("host"), "--port", fmt.Sprint(cmd.Int("port")), "--log-format", cmd.String("log-format"), "--log-level", cmd.String("log-level")}
	if db := cmd.String("db"); db != "" {
		args = append(args, "--db", db)
	}
	if consoleDevURL := cmd.String("console-dev-url"); consoleDevURL != "" {
		args = append(args, "--console-dev-url", consoleDevURL)
	}
	if cmd.Bool("ephemeral") {
		args = append(args, "--ephemeral")
	}
	for _, dir := range cmd.StringSlice("plugin-dir") {
		args = append(args, "--plugin-dir", dir)
	}
	if cmd.Bool("no-plugins") {
		args = append(args, "--no-plugins")
	}
	return args
}

func managedStateDir() (string, error) {
	return daemonstate.DefaultDir()
}

func waitForServiceReady(ctx context.Context, stateDir string) error {
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	registration, err := daemonclient.WaitReady(waitCtx, daemonstate.New(stateDir), version, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("Wingman service did not become ready; run 'wingman service status' for details: %w", err)
	}
	fmt.Printf("Wingman daemon ready at %s\n", registration.URL)
	return nil
}

func printDaemonStatus(ctx context.Context, state *daemonstate.State) {
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result := daemonclient.Inspect(probeCtx, state, version)
	switch result.Status {
	case daemonclient.StatusReady:
		fmt.Printf("Wingman daemon: ready at %s (%s)\n", result.Registration.URL, result.Registration.InstanceID)
	case daemonclient.StatusStarting:
		fmt.Printf("Wingman daemon: starting at %s\n", result.Registration.URL)
	case daemonclient.StatusIncompatible:
		fmt.Printf("Wingman daemon: incompatible version %s at %s\n", result.Registration.Version, result.Registration.URL)
	case daemonclient.StatusStale:
		fmt.Printf("Wingman daemon: stale registration at %s\n", result.Registration.URL)
	default:
		fmt.Println("Wingman daemon: no registration")
	}
}

func systemdUnit(home, configHome, configPath string, args []string) string {
	environment := "Environment=" + strconv.Quote("HOME="+home) + "\n"
	if configHome != "" {
		environment += "Environment=" + strconv.Quote("XDG_CONFIG_HOME="+configHome) + "\n"
	}
	return fmt.Sprintf("[Unit]\nDescription=Wingman agent harness\n\n[Service]\nType=simple\n%sEnvironmentFile=%s\nExecStart=%s\nRestart=on-failure\nRestartSec=2s\n\n[Install]\nWantedBy=default.target\n", environment, strconv.Quote(configPath), systemdCommand(args))
}

func systemdUnitPath(home, configHome string) string {
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "systemd", "user", systemdServiceName)
}

func systemdCommand(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = strconv.Quote(arg)
	}
	return strings.Join(quoted, " ")
}

func launchdDomain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }
func launchdTarget() string { return launchdDomain() + "/" + launchdLabel }
func launchdPlistPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func launchdPlist(home, configHome, configPath string, args []string) string {
	var programArgs strings.Builder
	for _, arg := range append([]string{"/bin/sh", "-c", `set -a; . "$1"; shift; exec "$@"`, "wingman-launch", configPath}, args...) {
		fmt.Fprintf(&programArgs, "\t\t<string>%s</string>\n", xmlEscape(arg))
	}
	var configEnvironment string
	if configHome != "" {
		configEnvironment = fmt.Sprintf("\t\t<key>XDG_CONFIG_HOME</key>\n\t\t<string>%s</string>\n", xmlEscape(configHome))
	}
	return fmt.Sprintf("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\">\n<dict>\n\t<key>Label</key>\n\t<string>%s</string>\n\t<key>ProgramArguments</key>\n\t<array>\n%s\t</array>\n\t<key>EnvironmentVariables</key>\n\t<dict>\n\t\t<key>HOME</key>\n\t\t<string>%s</string>\n%s\t</dict>\n\t<key>RunAtLoad</key>\n\t<true/>\n\t<key>KeepAlive</key>\n\t<true/>\n</dict>\n</plist>\n", launchdLabel, programArgs.String(), xmlEscape(home), configEnvironment)
}

func xmlEscape(value string) string {
	var escaped strings.Builder
	_ = xml.EscapeText(&escaped, []byte(value))
	return escaped.String()
}

func runSystemctl(ctx context.Context, args ...string) error {
	return runCommand(ctx, "systemctl", append([]string{"--user"}, args...)...)
}
func runLaunchctl(ctx context.Context, args ...string) error {
	return runCommand(ctx, "launchctl", args...)
}

func runCommand(ctx context.Context, name string, args ...string) error {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runSystemctlAttached(ctx context.Context, args ...string) error {
	return runCommandAttached(ctx, "systemctl", append([]string{"--user"}, args...)...)
}
func runLaunchctlAttached(ctx context.Context, args ...string) error {
	return runCommandAttached(ctx, "launchctl", args...)
}

func runCommandAttached(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
