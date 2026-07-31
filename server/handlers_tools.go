package server

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/chaserensberger/wingman/tool"
)

type toolCatalogResponse struct {
	Tools []toolCatalogItem `json:"tools"`
}

type toolCatalogItem struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description,omitempty"`
	InputSchema     map[string]any         `json:"input_schema,omitempty"`
	OutputSchema    map[string]any         `json:"output_schema,omitempty"`
	Sequential      bool                   `json:"sequential,omitempty"`
	DirectoryScoped bool                   `json:"directory_scoped,omitempty"`
	Permission      *tool.PermissionTarget `json:"permission,omitempty"`
	Source          string                 `json:"source"`
	Plugin          string                 `json:"plugin,omitempty"`
	Server          string                 `json:"server,omitempty"`
	RemoteName      string                 `json:"remote_name,omitempty"`
	Status          string                 `json:"status,omitempty"`
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	_, items, err := s.toolCatalog()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toolCatalogResponse{Tools: items})
}

func (s *Server) toolCatalog() (*tool.Registry, []toolCatalogItem, error) {
	var tools []tool.Tool
	items := make(map[string]toolCatalogItem)
	add := func(t tool.Tool, item toolCatalogItem) {
		tools = append(tools, t)
		if _, exists := items[t.Name()]; !exists {
			items[t.Name()] = item
		}
	}

	native := nativeTools()
	names := make([]string, 0, len(native))
	for name := range native {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		add(native[name], catalogItem(native[name], "native"))
	}

	if s.plugins != nil {
		owners := map[string]string{}
		plugins, _ := s.plugins.Status()
		for _, plugin := range plugins {
			if !plugin.Running {
				continue
			}
			for _, name := range plugin.Tools {
				owners[name] = plugin.ID
			}
		}
		pluginTools := s.plugins.Tools()
		sort.Slice(pluginTools, func(i, j int) bool { return pluginTools[i].Name() < pluginTools[j].Name() })
		for _, t := range pluginTools {
			item := catalogItem(t, "plugin")
			item.Plugin = owners[t.Name()]
			add(t, item)
		}
	}

	if s.mcp != nil {
		infos := make(map[string]toolCatalogItem)
		for _, info := range s.mcp.ToolInfos() {
			if info.Status != "connected" {
				continue
			}
			infos[info.Name] = toolCatalogItem{
				Name: info.Name, Description: info.Description, InputSchema: info.InputSchema,
				OutputSchema: info.OutputSchema, Source: info.Source, Server: info.Server,
				RemoteName: info.RemoteName, Status: info.Status,
			}
		}
		for _, t := range s.mcp.Tools() {
			item, ok := infos[t.Name()]
			if !ok {
				item = catalogItem(t, "mcp")
			}
			add(t, item)
		}
	}

	registry, err := tool.Compose(tools)
	if err != nil {
		return nil, nil, fmt.Errorf("compose tool catalog: %w", err)
	}
	ordered := make([]toolCatalogItem, 0, len(items))
	for _, t := range registry.List() {
		ordered = append(ordered, items[t.Name()])
	}
	return registry, ordered, nil
}

func catalogItem(t tool.Tool, source string) toolCatalogItem {
	def := t.Definition()
	return toolCatalogItem{
		Name: t.Name(), Description: t.Description(), InputSchema: def.AsModelToolDef().InputSchema,
		OutputSchema: def.OutputSchema, Sequential: tool.IsSequential(t),
		DirectoryScoped: tool.IsDirectoryScoped(t), Permission: def.Permission,
		Source: source, Status: "available",
	}
}
