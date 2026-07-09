package server

import (
	"net/http"
	"sort"

	"github.com/chaserensberger/wingman/tool"
)

type toolCatalogResponse struct {
	Tools []toolCatalogItem `json:"tools"`
}

type toolCatalogItem struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	Source      string         `json:"source"`
	Plugin      string         `json:"plugin,omitempty"`
	Server      string         `json:"server,omitempty"`
	RemoteName  string         `json:"remote_name,omitempty"`
	Status      string         `json:"status,omitempty"`
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	items := make([]toolCatalogItem, 0)
	for _, t := range nativeTools() {
		items = append(items, catalogItem(t, "native"))
	}
	if s.plugins != nil {
		plugins, _ := s.plugins.Status()
		owners := map[string]string{}
		for _, plugin := range plugins {
			for _, name := range plugin.Tools {
				owners[name] = plugin.ID
			}
		}
		for _, t := range s.plugins.Tools() {
			item := catalogItem(t, "plugin")
			item.Plugin = owners[t.Name()]
			items = append(items, item)
		}
	}
	if s.mcp != nil {
		for _, info := range s.mcp.ToolInfos() {
			items = append(items, toolCatalogItem{
				Name:        info.Name,
				Description: info.Description,
				InputSchema: info.InputSchema,
				Source:      info.Source,
				Server:      info.Server,
				RemoteName:  info.RemoteName,
				Status:      info.Status,
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Source != items[j].Source {
			return items[i].Source < items[j].Source
		}
		return items[i].Name < items[j].Name
	})
	writeJSON(w, http.StatusOK, toolCatalogResponse{Tools: items})
}

func catalogItem(t tool.Tool, source string) toolCatalogItem {
	def := t.Definition()
	return toolCatalogItem{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: def.AsModelToolDef().InputSchema,
		Source:      source,
		Status:      "available",
	}
}
