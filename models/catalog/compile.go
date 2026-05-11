package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

var yearMonthRe = regexp.MustCompile(`(?m)(=\s*)(\d{4}-\d{2})\s*$`)

type compileResult struct {
	Labs           []Lab
	LabModels      []LabModel
	Providers      []Provider
	ProviderModels []ProviderModel
}

func CompileDir(dataDir string) (*Snapshot, error) {
	result := &compileResult{}
	var errs []string

	err := filepath.WalkDir(dataDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".toml") {
			return nil
		}

		rel, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		kind := catalogDataKind(rel)
		if kind == "" {
			errs = append(errs, fmt.Sprintf("unknown catalog data file: %s", rel))
			return nil
		}

		if err := loadCatalogData(path, kind, result); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", rel, err))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk catalog data: %w", err)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("catalog data errors:\n  %s", strings.Join(errs, "\n  "))
	}

	sort.Slice(result.Labs, func(i, j int) bool { return result.Labs[i].ID < result.Labs[j].ID })
	sort.Slice(result.LabModels, func(i, j int) bool { return result.LabModels[i].ID < result.LabModels[j].ID })
	sort.Slice(result.Providers, func(i, j int) bool { return result.Providers[i].ID < result.Providers[j].ID })
	sort.Slice(result.ProviderModels, func(i, j int) bool { return result.ProviderModels[i].ID < result.ProviderModels[j].ID })

	return &Snapshot{
		Version:        "v1",
		GeneratedAt:    time.Now().UTC(),
		Labs:           result.Labs,
		LabModels:      result.LabModels,
		Providers:      result.Providers,
		ProviderModels: result.ProviderModels,
	}, nil
}

func WriteSnapshot(path string, snapshot *Snapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func catalogDataKind(rel string) string {
	parts := strings.Split(rel, "/")
	switch {
	case len(parts) == 3 && parts[0] == "labs" && parts[2] == "lab.toml":
		return "lab"
	case len(parts) == 4 && parts[0] == "labs" && parts[2] == "models":
		return "lab_model"
	case len(parts) == 3 && parts[0] == "providers" && parts[2] == "provider.toml":
		return "provider"
	case len(parts) == 4 && parts[0] == "providers" && parts[2] == "models":
		return "provider_model"
	default:
		return ""
	}
}

func loadCatalogData(path string, kind string, result *compileResult) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	content := yearMonthRe.ReplaceAllString(string(raw), `${1}"${2}"`)

	var m map[string]any
	if _, err := toml.Decode(content, &m); err != nil {
		return fmt.Errorf("decode TOML: %w", err)
	}
	convertDates(m)

	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal TOML map: %w", err)
	}

	switch kind {
	case "lab":
		var v Lab
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		result.Labs = append(result.Labs, v)
	case "lab_model":
		var v LabModel
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		result.LabModels = append(result.LabModels, v)
	case "provider":
		var v Provider
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		result.Providers = append(result.Providers, v)
	case "provider_model":
		var v ProviderModel
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		result.ProviderModels = append(result.ProviderModels, v)
	}
	return nil
}

func convertDates(m map[string]any) {
	for k, v := range m {
		switch val := v.(type) {
		case time.Time:
			m[k] = val.Format("2006-01-02")
		case map[string]any:
			convertDates(val)
		case []map[string]any:
			for _, item := range val {
				convertDates(item)
			}
		case []any:
			for _, item := range val {
				if sub, ok := item.(map[string]any); ok {
					convertDates(sub)
				}
			}
		}
	}
}
