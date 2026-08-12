package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/urfave/cli/v3"
	"golang.org/x/mod/semver"
)

const githubAPI = "https://api.github.com/repos/chaserensberger/wingman/releases"

var releaseAPI = githubAPI

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

func updateFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "version", Usage: "Install a specific release version"},
		&cli.BoolFlag{Name: "check", Usage: "Check whether an update is available without installing it"},
	}
}

func runUpdate(ctx context.Context, cmd *cli.Command) error {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return fmt.Errorf("wingman update supports Linux and macOS only")
	}

	requestedVersion := cmd.String("version")
	release, err := fetchRelease(ctx, http.DefaultClient, requestedVersion)
	if err != nil {
		return err
	}
	available, err := updateAvailable(version, release.TagName)
	if err != nil {
		return err
	}
	if cmd.Bool("check") {
		if !available {
			fmt.Printf("Wingman %s is already up to date\n", version)
			return nil
		}
		fmt.Printf("Wingman %s is available (current: %s)\n", release.TagName, version)
		return nil
	}
	if !shouldInstallUpdate(requestedVersion, available) {
		fmt.Printf("Wingman %s is already up to date\n", version)
		return nil
	}

	exe, err := executablePath()
	if err != nil {
		return err
	}
	if err := installRelease(ctx, http.DefaultClient, release, exe); err != nil {
		return err
	}
	fmt.Printf("Updated Wingman to %s\n", release.TagName)
	if err := restartManagedService(ctx); err != nil {
		return err
	}
	return nil
}

func fetchRelease(ctx context.Context, client *http.Client, requestedVersion string) (release, error) {
	url := releaseAPI + "/latest"
	if requestedVersion != "" {
		url = releaseAPI + "/tags/v" + strings.TrimPrefix(requestedVersion, "v")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return release{}, fmt.Errorf("create release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return release{}, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("fetch release: GitHub returned %s", resp.Status)
	}
	var result release
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return release{}, fmt.Errorf("decode release: %w", err)
	}
	if !semver.IsValid(normalizeVersion(result.TagName)) {
		return release{}, fmt.Errorf("release tag %q is not a semantic version", result.TagName)
	}
	return result, nil
}

func updateAvailable(current, latest string) (bool, error) {
	if current == "dev" {
		return true, nil
	}
	current = normalizeVersion(current)
	latest = normalizeVersion(latest)
	if !semver.IsValid(current) {
		return false, fmt.Errorf("current version %q is not a semantic version", version)
	}
	if !semver.IsValid(latest) {
		return false, fmt.Errorf("latest version %q is not a semantic version", latest)
	}
	return semver.Compare(current, latest) < 0, nil
}

func normalizeVersion(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

func shouldInstallUpdate(requestedVersion string, available bool) bool {
	return requestedVersion != "" || available
}

func installRelease(ctx context.Context, client *http.Client, release release, exe string) error {
	archiveName := fmt.Sprintf("wingman_%s_%s_%s.tar.gz", strings.TrimPrefix(release.TagName, "v"), runtime.GOOS, runtime.GOARCH)
	archive, checksums, err := releaseAssets(release, archiveName)
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	tmpDir, err := os.MkdirTemp(dir, ".wingman-update-")
	if err != nil {
		return fmt.Errorf("create update directory in %s: %w", dir, err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, archiveName)
	if err := downloadFile(ctx, client, archive.DownloadURL, archivePath); err != nil {
		return err
	}
	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadFile(ctx, client, checksums.DownloadURL, checksumsPath); err != nil {
		return err
	}
	if err := verifyChecksum(archivePath, checksumsPath, archiveName); err != nil {
		return err
	}

	newBinary := filepath.Join(tmpDir, "wingman")
	if err := extractBinary(archivePath, newBinary); err != nil {
		return err
	}
	info, err := os.Stat(exe)
	if err != nil {
		return fmt.Errorf("stat current binary: %w", err)
	}
	if err := os.Chmod(newBinary, info.Mode().Perm()); err != nil {
		return fmt.Errorf("set binary permissions: %w", err)
	}
	if err := os.Rename(newBinary, exe); err != nil {
		return fmt.Errorf("replace %s: %w", exe, err)
	}
	return nil
}

func releaseAssets(release release, archiveName string) (releaseAsset, releaseAsset, error) {
	var archive, checksums releaseAsset
	for _, asset := range release.Assets {
		switch asset.Name {
		case archiveName:
			archive = asset
		case "checksums.txt":
			checksums = asset
		}
	}
	if archive.DownloadURL == "" {
		return releaseAsset{}, releaseAsset{}, fmt.Errorf("release %s has no %s asset", release.TagName, archiveName)
	}
	if checksums.DownloadURL == "" {
		return releaseAsset{}, releaseAsset{}, fmt.Errorf("release %s has no checksums.txt asset", release.TagName)
	}
	return archive, checksums, nil
}

func downloadFile(ctx context.Context, client *http.Client, url, destination string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: server returned %s", url, resp.Status)
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create download %s: %w", destination, err)
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("write download %s: %w", destination, err)
	}
	return nil
}

func verifyChecksum(archivePath, checksumsPath, archiveName string) error {
	checksums, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}
	var expected string
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == archiveName {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("checksums.txt has no checksum for %s", archiveName)
	}
	if _, err := hex.DecodeString(expected); err != nil || len(expected) != sha256.Size*2 {
		return fmt.Errorf("checksums.txt has an invalid SHA-256 checksum for %s", archiveName)
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash archive: %w", err)
	}
	if !strings.EqualFold(expected, hex.EncodeToString(hash.Sum(nil))) {
		return errors.New("downloaded archive checksum does not match checksums.txt")
	}
	return nil
}

func extractBinary(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("read gzip archive: %w", err)
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		if header.Name != "wingman" || header.Typeflag != tar.TypeReg {
			continue
		}
		output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
		if err != nil {
			return fmt.Errorf("create replacement binary: %w", err)
		}
		_, copyErr := io.Copy(output, reader)
		closeErr := output.Close()
		if copyErr != nil {
			return fmt.Errorf("extract replacement binary: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close replacement binary: %w", closeErr)
		}
		return nil
	}
	return errors.New("archive does not contain the wingman binary")
}

func restartManagedService(ctx context.Context) error {
	switch runtime.GOOS {
	case "linux":
		if err := runSystemctl(ctx, "is-active", "--quiet", "wingman.service"); err != nil {
			return nil
		}
		if err := runSystemctl(ctx, "restart", "wingman.service"); err != nil {
			return fmt.Errorf("restart updated service: %w", err)
		}
		fmt.Println("Restarted Wingman service")
	case "darwin":
		running, err := launchdServiceRunning(ctx)
		if err != nil || !running {
			return nil
		}
		if err := runLaunchctl(ctx, "kickstart", "-k", launchdTarget()); err != nil {
			return fmt.Errorf("restart updated service: %w", err)
		}
		fmt.Println("Restarted Wingman service")
	}
	return nil
}

func launchdServiceRunning(ctx context.Context) (bool, error) {
	output, err := exec.CommandContext(ctx, "launchctl", "print", launchdTarget()).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("launchctl print %s: %w\n%s", launchdTarget(), err, strings.TrimSpace(string(output)))
	}
	return strings.Contains(string(output), "state = running"), nil
}
