// Package pluginhost discovers and runs out-of-process Wingman plugins.
package pluginhost

import (
	"context"
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

// Manifest declares the bootstrap data for one out-of-process plugin executable.
// Commands are executed directly; shell expansion is not applied.
type Manifest struct {
	ID      string         `json:"id"`
	Name    string         `json:"name,omitempty"`
	Command []string       `json:"command"`
	Config  map[string]any `json:"config,omitempty"`
	Path    string         `json:"-"`
}

// ToolSpec is the manifest shape for a plugin-contributed LLM tool.
type ToolSpec struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	InputSchema     map[string]any         `json:"input_schema"`
	OutputSchema    map[string]any         `json:"output_schema,omitempty"`
	Sequential      bool                   `json:"sequential,omitempty"`
	DirectoryScoped bool                   `json:"directory_scoped,omitempty"`
	Permission      *tool.PermissionTarget `json:"permission,omitempty"`
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
	return nil
}

func validateToolSpecs(specs []ToolSpec) error {
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if spec.Name == "" {
			return fmt.Errorf("tool name is required")
		}
		if spec.Description == "" {
			return fmt.Errorf("tool %q description is required", spec.Name)
		}
		if spec.InputSchema["type"] != "object" {
			return fmt.Errorf("tool %q input_schema.type must be object", spec.Name)
		}
		if _, exists := seen[spec.Name]; exists {
			return fmt.Errorf("duplicate tool name %q", spec.Name)
		}
		seen[spec.Name] = struct{}{}
		if err := tool.Validate(&manifestTool{spec: spec}); err != nil {
			return err
		}
	}
	return nil
}

type manifestTool struct{ spec ToolSpec }

func (t *manifestTool) Name() string        { return t.spec.Name }
func (t *manifestTool) Description() string { return t.spec.Description }
func (t *manifestTool) Definition() tool.Definition {
	return tool.Definition{
		Name: t.spec.Name, Description: t.spec.Description,
		InputSchema: tool.InputSchema{Type: "object"}, RawInputSchema: t.spec.InputSchema,
		OutputSchema: t.spec.OutputSchema, Sequential: t.spec.Sequential,
		DirectoryScoped: t.spec.DirectoryScoped, Permission: t.spec.Permission,
	}
}
func (*manifestTool) Execute(context.Context, tool.Invocation) (tool.Result, error) {
	return tool.Result{}, nil
}

// LocalPluginDir returns the project-local plugin directory for workDir.
func LocalPluginDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	return filepath.Join(workDir, localDirName, "plugins")
}
