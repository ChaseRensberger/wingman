package tool

import "github.com/chaserensberger/wingman/permission"

// WhollyDenied reports whether a tool's static permission action is fully denied.
func WhollyDenied(t Tool, rules permission.Ruleset) bool {
	def := t.Definition()
	action := t.Name()
	if _, dynamic := t.(PermissionedTool); dynamic {
		if _, patch := t.(*ApplyPatchTool); patch {
			return permission.WhollyDenied("edit", rules)
		}
		// A custom permission implementation may select its action from input.
		return false
	}
	if def.Permission != nil {
		action = def.Permission.Action
	} else {
		switch action {
		case "write", "edit", "apply_patch":
			action = "edit"
		}
	}
	return permission.WhollyDenied(action, rules)
}
