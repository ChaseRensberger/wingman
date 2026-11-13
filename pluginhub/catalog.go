// Package pluginhub resolves and installs versioned native plugin releases.
package pluginhub

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/chaserensberger/wingman/plugin/protocol"
	"golang.org/x/mod/semver"
)

// Catalog is a publisher-qualified index of immutable release artifacts.
type Catalog struct {
	Version  int       `json:"version"`
	Packages []Package `json:"packages"`
}

// Package identifies a publisher's plugin and its releases.
type Package struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Repository  string    `json:"repository"`
	Releases    []Release `json:"releases"`
}

// Release identifies an exact plugin version and its supported platforms.
type Release struct {
	Version        string              `json:"version"`
	Protocol       int                 `json:"protocol"`
	MinimumWingman string              `json:"minimum_wingman,omitempty"`
	Artifacts      map[string]Artifact `json:"artifacts"`
}

// Artifact is a precompiled executable served over HTTPS.
type Artifact struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

var packageID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._-]*$`)

// Validate rejects ambiguous identities, mutable versions, and unverifiable artifacts.
func (c Catalog) Validate() error {
	if c.Version != 1 {
		return errors.New("unsupported plugin catalog version")
	}
	seen := map[string]bool{}
	for _, p := range c.Packages {
		if !packageID.MatchString(p.ID) || seen[p.ID] {
			return fmt.Errorf("invalid or duplicate package %q", p.ID)
		}
		seen[p.ID] = true
		versions := map[string]bool{}
		for _, release := range p.Releases {
			if !semver.IsValid(release.Version) || versions[release.Version] {
				return fmt.Errorf("invalid or duplicate release %q for %s", release.Version, p.ID)
			}
			versions[release.Version] = true
			if release.Protocol <= 0 {
				return fmt.Errorf("invalid plugin protocol %d", release.Protocol)
			}
			if release.MinimumWingman != "" && !semver.IsValid(release.MinimumWingman) {
				return errors.New("minimum_wingman must be a semantic version")
			}
			if len(release.Artifacts) == 0 {
				return fmt.Errorf("release %s has no artifacts", release.Version)
			}
			for platform, artifact := range release.Artifacts {
				if !strings.Contains(platform, "/") {
					return fmt.Errorf("invalid platform %q", platform)
				}
				if err := secureURL(artifact.URL); err != nil {
					return err
				}
				checksum, err := hex.DecodeString(artifact.SHA256)
				if err != nil || len(checksum) != 32 {
					return fmt.Errorf("invalid SHA-256 for %s", platform)
				}
			}
		}
	}
	return nil
}

// Fetch reads a bounded catalog from an explicitly configured HTTPS endpoint.
func Fetch(ctx context.Context, client *http.Client, endpoint string) (Catalog, error) {
	if err := secureURL(endpoint); err != nil {
		return Catalog{}, err
	}
	data, err := download(ctx, client, endpoint, 4<<20)
	if err != nil {
		return Catalog{}, err
	}
	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, catalog.Validate()
}

// Search finds packages by identity, name, or description.
func (c Catalog) Search(query string) []Package {
	query = strings.ToLower(query)
	result := []Package{}
	for _, p := range c.Packages {
		if strings.Contains(strings.ToLower(p.ID+" "+p.Name+" "+p.Description), query) {
			result = append(result, p)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// Resolve chooses an exact compatible release without switching package sources.
func (c Catalog) Resolve(source, platform, wingmanVersion string) (Package, Release, Artifact, error) {
	if !semver.IsValid(wingmanVersion) && semver.IsValid("v"+wingmanVersion) {
		wingmanVersion = "v" + wingmanVersion
	}
	id, version, pinned := strings.Cut(source, "@")
	if !packageID.MatchString(id) {
		return Package{}, Release{}, Artifact{}, errors.New("plugin source must be publisher/name with an optional exact version")
	}
	if pinned && !semver.IsValid(version) {
		return Package{}, Release{}, Artifact{}, errors.New("plugin version must be an exact semantic version such as v1.2.0")
	}
	for _, p := range c.Packages {
		if p.ID == id {
			releases := append([]Release(nil), p.Releases...)
			sort.Slice(releases, func(i, j int) bool { return semver.Compare(releases[i].Version, releases[j].Version) > 0 })
			for _, release := range releases {
				if release.Protocol != protocol.Version {
					continue
				}
				if pinned && release.Version != version || !pinned && semver.Prerelease(release.Version) != "" {
					continue
				}
				if release.MinimumWingman != "" && (!semver.IsValid(wingmanVersion) || semver.Compare(wingmanVersion, release.MinimumWingman) < 0) {
					continue
				}
				artifact, ok := release.Artifacts[platform]
				if !ok {
					continue
				}
				return p, release, artifact, nil
			}
			return Package{}, Release{}, Artifact{}, fmt.Errorf("no compatible release of %s for Wingman %s on %s", source, wingmanVersion, platform)
		}
	}
	return Package{}, Release{}, Artifact{}, fmt.Errorf("plugin %q is not in this catalog", id)
}

func secureURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return errors.New("plugin catalogs and artifacts require an HTTPS URL")
	}
	return nil
}

func download(ctx context.Context, client *http.Client, endpoint string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// Never allow a catalog or artifact redirect to downgrade transport security.
	copyClient := *client
	previous := copyClient.CheckRedirect
	copyClient.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if err := secureURL(request.URL.String()); err != nil {
			return err
		}
		if len(via) >= 10 {
			return errors.New("too many plugin download redirects")
		}
		if previous != nil {
			return previous(request, via)
		}
		return nil
	}
	response, err := copyClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin download returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("plugin download exceeds size limit")
	}
	return data, nil
}
