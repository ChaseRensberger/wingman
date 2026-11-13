package pluginhub

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/gofrs/flock"
	"golang.org/x/mod/semver"
)

// Installation records the exact artifact selected for subsequent server starts.
type Installation struct {
	ID          string    `json:"id"`
	Version     string    `json:"version"`
	Requested   string    `json:"requested"`
	Catalog     string    `json:"catalog"`
	Artifact    Artifact  `json:"artifact"`
	Platform    string    `json:"platform"`
	InstalledAt time.Time `json:"installed_at"`
}

// Installer owns managed packages under one global plugin directory.
type Installer struct {
	Root           string
	CatalogURL     string
	WingmanVersion string
	Client         *http.Client
	Update         func(context.Context, func() error, func() error) error
}

func (i Installer) client() *http.Client {
	if i.Client != nil {
		return i.Client
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

// Catalog reads the configured public release catalog.
func (i Installer) Catalog(ctx context.Context) (Catalog, error) {
	if i.CatalogURL == "" {
		return Catalog{}, errors.New("configure plugins.catalog with the release catalog HTTPS URL")
	}
	return Fetch(ctx, i.client(), i.CatalogURL)
}

func (i Installer) lock(ctx context.Context) (*flock.Flock, error) {
	if i.Root == "" {
		return nil, errors.New("plugin installation directory is required")
	}
	if !filepath.IsAbs(i.Root) {
		return nil, errors.New("plugin installation directory must be absolute")
	}
	if err := os.MkdirAll(i.Root, 0o700); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(i.Root, ".install.lock"))
	locked, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, ctx.Err()
	}
	return lock, nil
}

func (i Installer) directory(id string) string {
	sum := sha256.Sum256([]byte(id))
	return filepath.Join(i.Root, "package-"+hex.EncodeToString(sum[:16]))
}

// List returns the managed installation records, without executing plugins.
func (i Installer) List() ([]Installation, error) {
	entries, err := os.ReadDir(i.Root)
	if os.IsNotExist(err) {
		return []Installation{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Installation{}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "package-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(i.Root, entry.Name(), "wingman-plugin.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var installation Installation
		var manifest pluginhost.Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, err
		}
		if len(manifest.Package) == 0 {
			continue
		}
		if err := json.Unmarshal(manifest.Package, &installation); err != nil {
			return nil, err
		}
		result = append(result, installation)
	}
	return result, nil
}

// Install validates a release before selecting it, preserving working files on failure.
func (i Installer) Install(ctx context.Context, source string, update bool) (Installation, error) {
	lock, err := i.lock(ctx)
	if err != nil {
		return Installation{}, err
	}
	defer lock.Unlock()
	catalog, err := i.Catalog(ctx)
	if err != nil {
		return Installation{}, err
	}
	p, release, artifact, err := catalog.Resolve(source, runtime.GOOS+"/"+runtime.GOARCH, i.WingmanVersion)
	if err != nil {
		return Installation{}, err
	}
	dir := i.directory(p.ID)
	oldRecord, readErr := os.ReadFile(filepath.Join(dir, "wingman-plugin.json"))
	var previous pluginhost.Manifest
	if readErr != nil && !os.IsNotExist(readErr) {
		return Installation{}, readErr
	}
	if readErr == nil {
		var old Installation
		if err := json.Unmarshal(oldRecord, &previous); err != nil {
			return Installation{}, err
		}
		if err := json.Unmarshal(previous.Package, &old); err != nil {
			return Installation{}, err
		}
		if !update {
			return Installation{}, fmt.Errorf("plugin %s is installed; use plugin update", p.ID)
		}
		if old.Catalog != i.CatalogURL {
			return Installation{}, errors.New("plugin updates must use the original catalog")
		}
		if strings.Contains(old.Requested, "@") && !strings.Contains(source, "@") {
			return Installation{}, errors.New("plugin is pinned; choose an explicit version to update it")
		}
		if old.Version == release.Version && old.Artifact.SHA256 != artifact.SHA256 {
			return Installation{}, errors.New("catalog changed an immutable release artifact")
		}
		if semver.Compare(release.Version, old.Version) < 0 && !strings.Contains(source, "@") {
			return Installation{}, errors.New("update would downgrade the plugin; choose an explicit version")
		}
	} else if update {
		return Installation{}, fmt.Errorf("plugin %s is not installed", p.ID)
	}
	data, err := download(ctx, i.client(), artifact.URL, 128<<20)
	if err != nil {
		return Installation{}, err
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), artifact.SHA256) {
		return Installation{}, errors.New("plugin artifact checksum mismatch")
	}
	// Artifacts are executables, not archives. Assets belong in the executable via go:embed.
	versions := filepath.Join(dir, ".versions", release.Version+"-"+hex.EncodeToString(sum[:8]))
	if err := os.MkdirAll(versions, 0o700); err != nil {
		return Installation{}, err
	}
	executable := filepath.Join(versions, "plugin")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	if _, err := os.Stat(executable); os.IsNotExist(err) {
		if err := atomicWriteMode(executable, data, 0o700); err != nil {
			return Installation{}, err
		}
	} else if err != nil {
		return Installation{}, err
	} else {
		existing, err := os.ReadFile(executable)
		if err != nil {
			return Installation{}, err
		}
		if sha256.Sum256(existing) != sum {
			return Installation{}, errors.New("installed plugin executable failed its integrity check")
		}
	}
	manifest := pluginhost.Manifest{ID: p.ID, Name: p.Name, Command: []string{executable}, Config: previous.Config, Disabled: previous.Disabled}
	staging, err := os.MkdirTemp(i.Root, ".validate-")
	if err != nil {
		return Installation{}, err
	}
	defer os.RemoveAll(staging)
	validationManifest := manifest
	validationManifest.Disabled = false
	if err := writeJSON(filepath.Join(staging, "wingman-plugin.json"), validationManifest); err != nil {
		return Installation{}, err
	}
	validation, err := pluginhost.New(ctx, []string{staging})
	if err != nil {
		return Installation{}, fmt.Errorf("validate plugin: %w", err)
	}
	statuses, _ := validation.Status()
	closeErr := validation.Close()
	if closeErr != nil {
		return Installation{}, closeErr
	}
	if len(statuses) != 1 || statuses[0].PluginVersion != release.Version {
		return Installation{}, errors.New("plugin executable version does not match catalog release")
	}
	installation := Installation{ID: p.ID, Version: release.Version, Requested: source, Catalog: i.CatalogURL, Artifact: artifact, Platform: runtime.GOOS + "/" + runtime.GOARCH, InstalledAt: time.Now().UTC()}
	manifest.Package, err = json.Marshal(installation)
	if err != nil {
		return Installation{}, err
	}
	manifestPath := filepath.Join(dir, "wingman-plugin.json")
	oldManifest, err := os.ReadFile(manifestPath)
	if err != nil && !os.IsNotExist(err) {
		return Installation{}, err
	}
	if err := i.activate(ctx, func() error { return writeJSON(manifestPath, manifest) }, func() error {
		if len(oldManifest) > 0 {
			return atomicWrite(manifestPath, oldManifest)
		}
		return os.Remove(manifestPath)
	}); err != nil {
		return Installation{}, fmt.Errorf("activate plugin: %w", err)
	}
	return installation, nil
}

// Remove disables a managed package without deleting versions still used by active runs.
func (i Installer) Remove(ctx context.Context, id string) error {
	if !packageID.MatchString(id) {
		return errors.New("plugin ID must be publisher/name")
	}
	lock, err := i.lock(ctx)
	if err != nil {
		return err
	}
	defer lock.Unlock()
	dir := i.directory(id)
	manifestPath := filepath.Join(dir, "wingman-plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest pluginhost.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil || len(manifest.Package) == 0 {
		return errors.New("plugin is not a managed installation")
	}
	return i.activate(ctx, func() error { return os.Remove(manifestPath) }, func() error { return atomicWrite(manifestPath, data) })
}

func (i Installer) activate(ctx context.Context, change, rollback func() error) error {
	if i.Update != nil {
		return i.Update(ctx, change, rollback)
	}
	return change()
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'))
}

func atomicWrite(path string, data []byte) error {
	return atomicWriteMode(path, data, 0o600)
}

func atomicWriteMode(path string, data []byte, mode os.FileMode) error {
	return atomicWriteReader(path, bytes.NewReader(data), mode)
}

func atomicWriteReader(path string, reader io.Reader, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".install-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, reader); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
