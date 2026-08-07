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
	systemdServicePath = "/etc/systemd/system/wingman.service"
	systemdStateDir    = "/var/lib/wingman"
	systemdStateDirEnv = "${STATE_DIRECTORY}"
	launchdLabel       = "actor.wingman"
)

func runUp(ctx context.Context, cmd *cli.Command) error {
	switch runtime.GOOS {
	case "linux":
		return runLinuxUp(ctx, cmd)
	case "darwin":
		return runDarwinUp(ctx, cmd)
	default:
		return fmt.Errorf("wingman up supports Linux/systemd and macOS/launchd only")
	}
}

func runDown(ctx context.Context, cmd *cli.Command) error {
	switch runtime.GOOS {
	case "linux":
		return runLinuxDown(ctx)
	case "darwin":
		return runDarwinDown(ctx)
	default:
		return fmt.Errorf("wingman down supports Linux/systemd and macOS/launchd only")
	}
}

func runRestart(ctx context.Context, cmd *cli.Command) error {
	stateDir, err := managedStateDir()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "linux":
		ok, err := ensureSystemdRoot(ctx)
		if err != nil || !ok {
			return err
		}
		if err := runSystemctl(ctx, "restart", "wingman.service"); err != nil {
			return err
		}
	case "darwin":
		if err := runLaunchctl(ctx, "kickstart", "-k", launchdTarget()); err != nil {
			return err
		}
	default:
		return fmt.Errorf("wingman restart supports Linux/systemd and macOS/launchd only")
	}
	return waitForServiceReady(ctx, stateDir)
}

func runStatus(ctx context.Context, cmd *cli.Command) error {
	stateDir, err := managedStateDir()
	if err == nil {
		printDaemonStatus(ctx, daemonstate.New(stateDir))
	}
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("systemctl"); err != nil {
			return fmt.Errorf("systemctl not found: %w", err)
		}
		return runSystemctlAttached(ctx, "status", "wingman.service")
	case "darwin":
		return runLaunchctlAttached(ctx, "print", launchdTarget())
	default:
		return fmt.Errorf("wingman status supports Linux/systemd and macOS/launchd only")
	}
}

func runLinuxUp(ctx context.Context, cmd *cli.Command) error {
	ok, err := ensureSystemdRoot(ctx)
	if err != nil || !ok {
		return err
	}
	exe, err := executablePath()
	if err != nil {
		return err
	}
	serviceUser, homeDir, err := serviceAccount()
	if err != nil {
		return err
	}
	if err := os.WriteFile(systemdServicePath, []byte(systemdUnit(exe, serviceUser, homeDir, serveArgs(exe, cmd, systemdStateDirEnv))), 0644); err != nil {
		return fmt.Errorf("write %s: %w", systemdServicePath, err)
	}
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "enable", "wingman.service"); err != nil {
		return err
	}
	if err := runSystemctl(ctx, "restart", "wingman.service"); err != nil {
		return err
	}
	if err := waitForServiceReady(ctx, systemdStateDir); err != nil {
		return err
	}
	fmt.Println("Wingman service installed and started")
	fmt.Println("Run 'wingman status' to inspect it")
	return nil
}

func runLinuxDown(ctx context.Context) error {
	ok, err := ensureSystemdRoot(ctx)
	if err != nil || !ok {
		return err
	}
	if err := runSystemctl(ctx, "disable", "--now", "wingman.service"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if err := os.Remove(systemdServicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", systemdServicePath, err)
	}
	if err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	fmt.Println("Wingman service stopped and removed")
	return nil
}

func runDarwinUp(ctx context.Context, cmd *cli.Command) error {
	if os.Geteuid() == 0 {
		return fmt.Errorf("run wingman up as the logged-in user, not root")
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
	stateDir := stateDirForHome(home)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create LaunchAgents directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(launchdPlist(home, serveArgs(exe, cmd, stateDir))), 0644); err != nil {
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
	fmt.Println("Run 'wingman status' to inspect it")
	return nil
}

func runDarwinDown(ctx context.Context) error {
	if os.Geteuid() == 0 {
		return fmt.Errorf("run wingman down as the logged-in user, not root")
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

func ensureSystemdRoot(ctx context.Context) (bool, error) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false, fmt.Errorf("systemctl not found: %w", err)
	}
	if os.Geteuid() == 0 {
		return true, nil
	}
	exe, err := executablePath()
	if err != nil {
		return false, err
	}
	args := append([]string{exe}, os.Args[1:]...)
	if os.Getenv("XDG_STATE_HOME") != "" {
		args = append([]string{"--preserve-env=XDG_STATE_HOME"}, args...)
	}
	sudo := exec.CommandContext(ctx, "sudo", args...)
	sudo.Stdin, sudo.Stdout, sudo.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := sudo.Run(); err != nil {
		return false, fmt.Errorf("sudo %s: %w", strings.Join(args, " "), err)
	}
	return false, nil
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

func stateDirForHome(home string) string {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "wingman")
	}
	return filepath.Join(home, ".local", "state", "wingman")
}

func managedStateDir() (string, error) {
	if runtime.GOOS == "linux" {
		return systemdStateDir, nil
	}
	_, home, err := serviceAccount()
	if err != nil {
		return "", err
	}
	return stateDirForHome(home), nil
}

func waitForServiceReady(ctx context.Context, stateDir string) error {
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	registration, err := daemonclient.WaitReady(waitCtx, daemonstate.New(stateDir), version, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("Wingman service did not become ready; run 'wingman status' for details: %w", err)
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

func systemdUnit(exe, serviceUser, homeDir string, args []string) string {
	return fmt.Sprintf("[Unit]\nDescription=Wingman agent harness\n\n[Service]\nType=simple\nUser=%s\nEnvironment=%s\nStateDirectory=wingman\nStateDirectoryMode=0700\nExecStart=%s\nRestart=on-failure\nRestartSec=2s\n\n[Install]\nWantedBy=multi-user.target\n", serviceUser, strconv.Quote("HOME="+homeDir), systemdCommand(args))
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

func launchdPlist(home string, args []string) string {
	var programArgs strings.Builder
	for _, arg := range args {
		fmt.Fprintf(&programArgs, "\t\t<string>%s</string>\n", xmlEscape(arg))
	}
	return fmt.Sprintf("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\">\n<dict>\n\t<key>Label</key>\n\t<string>%s</string>\n\t<key>ProgramArguments</key>\n\t<array>\n%s\t</array>\n\t<key>EnvironmentVariables</key>\n\t<dict>\n\t\t<key>HOME</key>\n\t\t<string>%s</string>\n\t</dict>\n\t<key>RunAtLoad</key>\n\t<true/>\n\t<key>KeepAlive</key>\n\t<true/>\n</dict>\n</plist>\n", launchdLabel, programArgs.String(), xmlEscape(home))
}

func xmlEscape(value string) string {
	var escaped strings.Builder
	_ = xml.EscapeText(&escaped, []byte(value))
	return escaped.String()
}

func runSystemctl(ctx context.Context, args ...string) error {
	return runCommand(ctx, "systemctl", args...)
}

func runSystemctlRoot(ctx context.Context, args ...string) error {
	if os.Geteuid() == 0 {
		return runSystemctl(ctx, args...)
	}
	return runCommand(ctx, "sudo", append([]string{"systemctl"}, args...)...)
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
	return runCommandAttached(ctx, "systemctl", args...)
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
