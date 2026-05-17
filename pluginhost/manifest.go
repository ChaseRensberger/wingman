// Package pluginhost discovers and runs out-of-process Wingman plugins.
package pluginhost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chaserensberger/wingman/tool"
)

const (
	manifestName = "wingman-plugin.json"
	localDirName = ".wingman"
)

// Manifest declares one out-of-process plugin executable and the tools it
// contributes. Commands are executed directly; shell expansion is not applied.
type Manifest struct {
	ID      string     `json:"id"`
	Name    string     `json:"name,omitempty"`
	Command []string   `json:"command"`
	Tools   []ToolSpec `json:"tools,omitempty"`
	Path    string     `json:"-"`
}

// ToolSpec is the manifest shape for a plugin-contributed LLM tool.
type ToolSpec struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	InputSchema tool.InputSchema `json:"input_schema"`
}

func discoverManifests(dirs []string) ([]Manifest, []LoadError) {
	var manifests []Manifest
	var errs []LoadError
	seen := make(map[string]struct{})

	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		paths, err := manifestPaths(dir)
		if err != nil {
			errs = append(errs, LoadError{Path: dir, Error: err.Error()})
			continue
		}
		for _, p := range paths {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			m, err := readManifest(p)
			if err != nil {
				errs = append(errs, LoadError{Path: p, Error: err.Error()})
				continue
			}
			manifests = append(manifests, m)
		}
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].ID < manifests[j].ID })
	return manifests, errs
}

func manifestPaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		p := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			candidate := filepath.Join(p, manifestName)
			if _, err := os.Stat(candidate); err == nil {
				paths = append(paths, candidate)
			}
			continue
		}
		if entry.Name() == manifestName || strings.HasSuffix(entry.Name(), ".plugin.json") {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func readManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	m.Path = path
	if err := validateManifest(m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func validateManifest(m Manifest) error {
	if m.ID == "" {
		return fmt.Errorf("plugin id is required")
	}
	if len(m.Command) == 0 || m.Command[0] == "" {
		return fmt.Errorf("plugin command is required")
	}
	for _, spec := range m.Tools {
		if spec.Name == "" {
			return fmt.Errorf("tool name is required")
		}
		if spec.Description == "" {
			return fmt.Errorf("tool %q description is required", spec.Name)
		}
		if spec.InputSchema.Type == "" {
			return fmt.Errorf("tool %q input_schema.type is required", spec.Name)
		}
	}
	return nil
}

// LocalPluginDir returns the project-local plugin directory for workDir.
func LocalPluginDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	return filepath.Join(workDir, localDirName, "plugins")
}
