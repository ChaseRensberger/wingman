package tool

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const ripgrepVersion = "15.1.0"

type ripgrepRelease struct {
	target, extension, sha256 string
}

// Digests are pinned from the upstream 15.1.0 release assets, not the download response.
var ripgrepReleases = map[string]ripgrepRelease{
	"darwin/arm64":  {"aarch64-apple-darwin", "tar.gz", "378e973289176ca0c6054054ee7f631a065874a352bf43f0fa60ef079b6ba715"},
	"darwin/amd64":  {"x86_64-apple-darwin", "tar.gz", "64811cb24e77cac3057d6c40b63ac9becf9082eedd54ca411b475b755d334882"},
	"linux/arm64":   {"aarch64-unknown-linux-gnu", "tar.gz", "2b661c6ef508e902f388e9098d9c4c5aca72c87b55922d94abdba830b4dc885e"},
	"linux/amd64":   {"x86_64-unknown-linux-musl", "tar.gz", "1c9297be4a084eea7ecaedf93eb03d058d6faae29bbc57ecdaf5063921491599"},
	"windows/arm64": {"aarch64-pc-windows-msvc", "zip", "00d931fb5237c9696ca49308818edb76d8eb6fc132761cb2a1bd616b2df02f8e"},
	"windows/amd64": {"x86_64-pc-windows-msvc", "zip", "124510b94b6baa3380d051fdf4650eaa80a302c876d611e9dba0b2e18d87493a"},
}

func ripgrepBinary(ctx context.Context) (string, error) {
	if binary, err := exec.LookPath("rg"); err == nil {
		return binary, nil
	}
	platform := runtime.GOOS + "/" + runtime.GOARCH
	release, ok := ripgrepReleases[platform]
	var binary string
	var err error
	if !ok {
		err = fmt.Errorf("no automatic download for %s", platform)
	} else {
		var cache string
		cache, err = os.UserCacheDir()
		if err == nil {
			binary, err = (ripgrepInstaller{
				cache: cache, release: release, client: http.DefaultClient,
				baseURL: "https://github.com/BurntSushi/ripgrep/releases/download/" + ripgrepVersion,
			}).binary(ctx)
		}
	}
	if err != nil {
		return "", fmt.Errorf("cannot obtain ripgrep: %w; install ripgrep and ensure rg is on PATH", err)
	}
	return binary, nil
}

type ripgrepInstaller struct {
	cache   string
	release ripgrepRelease
	client  *http.Client
	baseURL string
}

func (r ripgrepInstaller) binary(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	name := "rg"
	if r.release.extension == "zip" {
		name += ".exe"
	}
	dir := filepath.Join(r.cache, "wingman", "bin", "ripgrep-"+ripgrepVersion+"-"+r.release.target)
	target := filepath.Join(dir, name)
	unlock, err := lockMutation(ctx, target)
	if err != nil {
		return "", err
	}
	defer unlock()
	if info, err := os.Stat(target); err == nil {
		if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0) {
			return "", fmt.Errorf("cached ripgrep is not executable: %s", target)
		}
		return target, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	stem := "ripgrep-" + ripgrepVersion + "-" + r.release.target
	url := r.baseURL + "/" + stem + "." + r.release.extension
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	response, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download ripgrep: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download ripgrep: HTTP %d", response.StatusCode)
	}
	const maxArchive = 32 << 20
	archive, err := io.ReadAll(io.LimitReader(response.Body, maxArchive+1))
	if err != nil {
		return "", fmt.Errorf("download ripgrep: %w", err)
	}
	if len(archive) > maxArchive {
		return "", fmt.Errorf("ripgrep archive exceeds 32 MiB")
	}
	if fmt.Sprintf("%x", sha256.Sum256(archive)) != r.release.sha256 {
		return "", fmt.Errorf("ripgrep archive checksum mismatch")
	}
	binary, err := ripgrepArchiveBinary(archive, r.release.extension, stem+"/"+name)
	if err != nil {
		return "", err
	}
	defer binary.Close()
	file, err := os.CreateTemp(dir, ".rg-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	const maxBinary = 64 << 20
	n, copyErr := io.Copy(file, io.LimitReader(binary, maxBinary+1))
	closeErr := file.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n == 0 || n > maxBinary {
		return "", fmt.Errorf("invalid ripgrep executable size: %d", n)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.Chmod(file.Name(), 0755); err != nil {
		return "", err
	}
	// Publish only a complete binary; other processes never see a partial download.
	if err := os.Rename(file.Name(), target); err != nil {
		return "", err
	}
	return target, nil
}

func ripgrepArchiveBinary(data []byte, extension, name string) (io.ReadCloser, error) {
	if extension == "zip" {
		archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, file := range archive.File {
			if file.Name == name && file.Mode().IsRegular() {
				return file.Open()
			}
		}
	} else {
		compressed, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		archive := tar.NewReader(compressed)
		for {
			header, err := archive.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				compressed.Close()
				return nil, err
			}
			if header.Name == name && header.Typeflag == tar.TypeReg {
				return struct {
					io.Reader
					io.Closer
				}{archive, compressed}, nil
			}
		}
		compressed.Close()
	}
	return nil, fmt.Errorf("ripgrep archive does not contain %s", name)
}
