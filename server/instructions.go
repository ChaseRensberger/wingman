package server

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/instruction"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/skill"
	"github.com/chaserensberger/wingman/store"
)

const agentsFileName = "AGENTS.md"

func (s *Server) resolveInstructions(agent *store.Agent, workDir string) (string, []store.InstructionSource, error) {
	resolved, err := instruction.Discover(context.Background(), s.globalInstructionsPath, workDir, nil)
	if err != nil {
		return "", nil, err
	}
	var sources []store.InstructionSource
	for _, source := range resolved {
		sources = append(sources, store.InstructionSource{
			Kind: source.Kind, Path: source.Path, SHA256: source.SHA256,
			ResolvedAt: time.Now().UTC(), Order: len(sources) + 1,
		})
	}
	base := agent.Instructions + "\n\nCurrent date: " + time.Now().Format(time.DateOnly) + "."
	return instruction.Render(base, resolved), sources, nil
}

func (s *Server) resolveSkills(agent *store.Agent, workDir string) ([]skill.Skill, string, error) {
	builtins, err := skill.Builtins()
	if err != nil {
		return nil, "", fmt.Errorf("load built-in skills: %w", err)
	}
	dirs := append([]string(nil), s.globalSkillDirs...)
	if workDir != "" {
		dirs = append(dirs, filepath.Join(workDir, ".wingman", "skills"))
	}
	discovered, err := skill.Discover(dirs...)
	if err != nil {
		return nil, "", fmt.Errorf("discover skills: %w", err)
	}
	byID := make(map[string]skill.Skill, len(builtins)+len(discovered))
	for _, entry := range builtins {
		byID[entry.ID] = entry
	}
	for _, entry := range discovered {
		byID[entry.ID] = entry
	}
	skills := make([]skill.Skill, 0, len(byID))
	for _, entry := range byID {
		skills = append(skills, entry)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].ID < skills[j].ID })
	var lines []string
	for _, entry := range skills {
		if entry.Description == "" || permission.Evaluate("skill", entry.ID, s.effectivePermissions(agent), permission.EffectAllow).Effect == permission.EffectDeny {
			continue
		}
		lines = append(lines, skillSummary(entry))
	}
	if len(lines) == 0 {
		return skills, "", nil
	}
	return skills, skillCatalogHeader + strings.Join(lines, "\n"), nil
}

func (s *Server) instructionResolver(ctx context.Context, data store.Store, sess *store.Session, agent *store.Agent, skills []skill.Skill) (run.InstructionResolver, error) {
	var reads []instruction.Read
	seen := make(map[string]bool)
	remember := func(metadata map[string]any, toolUseID, callID string) {
		resolved, _ := metadata["resolved_path"].(string)
		if resolved == "" || seen[resolved] {
			return
		}
		requested, _ := metadata["requested_path"].(string)
		reads = append(reads, instruction.Read{RequestedPath: requested, ResolvedPath: resolved, ToolUseID: toolUseID, CallID: callID})
		seen[resolved] = true
	}
	if data != nil {
		uses, err := data.ListToolUses(ctx, sess.ID)
		if err != nil {
			return nil, err
		}
		for _, use := range uses {
			if use.Name != "read" || use.Status != store.ToolUseStatusCompleted {
				continue
			}
			var metadata map[string]any
			if err := json.Unmarshal(use.MetadataJSON, &metadata); err != nil && len(use.MetadataJSON) > 0 {
				return nil, err
			}
			remember(metadata, use.ID, use.CallID)
		}
	}
	var catalog []string
	for _, entry := range skills {
		if entry.Description != "" && permission.Evaluate("skill", entry.ID, s.effectivePermissions(agent), permission.EffectAllow).Effect != permission.EffectDeny {
			catalog = append(catalog, skillSummary(entry))
		}
	}
	return func(ctx context.Context, system string, history []models.Message) (string, []models.InstructionSnapshot, error) {
		for _, message := range history {
			for _, part := range message.Content {
				switch value := part.(type) {
				case models.ToolPart:
					if value.Name == "read" && value.State == models.ToolStateCompleted {
						remember(value.Metadata, value.ToolUseID, value.CallID)
					}
				case models.ToolResultPart:
					if value.Name == "read" && !value.IsError {
						remember(value.Metadata, value.ToolUseID, value.CallID)
					}
				}
			}
		}
		sources, err := instruction.Discover(ctx, s.globalInstructionsPath, sess.WorkDir, reads)
		if err != nil {
			return "", nil, err
		}
		base := system + "\n\nCurrent date: " + time.Now().Format(time.DateOnly) + "."
		result := instruction.Render(base, sources)
		if len(catalog) > 0 {
			result += "\n\n" + skillCatalogHeader + strings.Join(catalog, "\n")
		}
		return result, sources, nil
	}, nil
}

const skillCatalogHeader = "## Available skills\nUse the skill tool with an ID to load instructions when the task matches the description. Do not load a skill based only on keywords or availability. Avoid reloading instructions already in context.\n"

func skillSummary(entry skill.Skill) string {
	return fmt.Sprintf("- %s (%s): %s", entry.ID, entry.Name, entry.Description)
}
