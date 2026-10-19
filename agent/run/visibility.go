package run

import (
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/tool"
)

func (r *runner) visibleToolDefs(defs []models.ToolDef) []models.ToolDef {
	var out []models.ToolDef
	for _, def := range defs {
		t, err := r.registry.Get(def.Name)
		if err == nil && tool.WhollyDenied(t, r.cfg.Permissions) {
			continue
		}
		if err != nil && permission.WhollyDenied(def.Name, r.cfg.Permissions) {
			continue
		}
		out = append(out, def)
	}
	return out
}
