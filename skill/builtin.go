package skill

import (
	"crypto/sha256"
	_ "embed"
	"fmt"

	"github.com/chaserensberger/wingman/tool"
	wingmandocs "github.com/chaserensberger/wingman/web/apps/docs"
)

const (
	builtinWingSkillLocation  = "builtin/wingskill/SKILL.md"
	wingmanDocsResourceSource = "wingman-docs"
)

//go:embed builtin/wingskill/SKILL.md
var builtinWingSkill []byte

// Builtins returns the skills bundled with Wingman.
func Builtins() ([]Skill, error) {
	name, description, content, err := parse(builtinWingSkill)
	if err != nil {
		return nil, fmt.Errorf("parse built-in skill %q: %w", builtinWingSkillLocation, err)
	}
	docs, err := wingmandocs.Files()
	if err != nil {
		return nil, fmt.Errorf("list bundled documentation: %w", err)
	}
	resources := make([]EmbeddedResource, len(docs))
	for i, file := range docs {
		resources[i] = EmbeddedResource{Path: file.Path, SHA256: file.SHA256}
	}
	sum := sha256.Sum256(builtinWingSkill)
	return []Skill{{
		ID:                     "wingskill",
		Name:                   name,
		Description:            description,
		Content:                content,
		Location:               builtinWingSkillLocation,
		BaseDir:                "builtin/wingskill",
		SHA256:                 fmt.Sprintf("%x", sum),
		EmbeddedResourceSource: wingmanDocsResourceSource,
		EmbeddedResources:      resources,
	}}, nil
}

func readEmbeddedResource(s Skill, path string) (tool.Result, error) {
	if s.EmbeddedResourceSource != wingmanDocsResourceSource {
		return tool.Result{}, fmt.Errorf("embedded resource source %q is not available", s.EmbeddedResourceSource)
	}
	file, err := wingmandocs.Read(path)
	if err != nil {
		return tool.Result{}, fmt.Errorf("read bundled documentation %q: %w", path, err)
	}
	return tool.Result{Text: fmt.Sprintf("<skill_file skill=%q path=%q>\n%s\n</skill_file>", s.ID, path, file.Content)}, nil
}
