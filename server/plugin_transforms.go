package server

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"

	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/plugins/orientation"
	"github.com/chaserensberger/wingman/skill"
	"github.com/chaserensberger/wingman/store"
)

type sessionDraft struct {
	PluginAgent bool
	Agent       store.Agent
	Skills      []skill.Skill
}

func (s *Server) transformAgent(ctx context.Context, activation *pluginhost.Activation, selected *store.Agent, pluginAgent bool) (*store.Agent, error) {
	seed := selected
	if pluginAgent {
		seed = nil
	}
	result, err := s.transformedAgents(ctx, activation, seed)
	if err != nil {
		return nil, err
	}
	agent, ok := result[selected.ID]
	if !ok {
		return nil, fmt.Errorf("agent %q is unavailable after plugin transforms", selected.ID)
	}
	if pluginAgent {
		route, ok, err := modelRouteFromOptions(selected.Options)
		if err != nil {
			return nil, err
		}
		if ok {
			return s.agentWithRequestModel(&agent, selected.ModelRef, &route), nil
		}
		return s.agentWithRequestModel(&agent, selected.ModelRef, nil), nil
	}
	return &agent, nil
}

func (s *Server) transformedAgents(ctx context.Context, activation *pluginhost.Activation, selected *store.Agent) (protocol.Agents, error) {
	agents := protocol.Agents{}
	if s.store != nil {
		all, err := s.store.ListAgents()
		if err != nil {
			return nil, err
		}
		for _, agent := range all {
			agents[agent.ID] = *agent
		}
	}
	if selected != nil {
		agents[selected.ID] = *selected
	}
	if activation == nil || !activation.HasTransforms() {
		return agents, nil
	}
	var result protocol.Agents
	if err := activation.Transform(ctx, "agents", agents, &result); err != nil {
		return nil, err
	}
	for id, agent := range result {
		if (id == "" && (selected == nil || selected.ID != "")) || agent.ID != id {
			return nil, fmt.Errorf("agent identity does not match key %q", id)
		}
	}
	return result, nil
}

func (s *Server) agentForRun(r *http.Request, directory, id string) (*store.Agent, bool, error) {
	if agent, err := s.store.GetAgent(id); err == nil {
		return agent, false, nil
	}
	activation, release, err := s.requestPluginActivation(r, directory)
	if err != nil {
		return nil, false, err
	}
	defer release()
	draft, err := s.transformedAgents(r.Context(), activation, nil)
	if err != nil {
		return nil, false, err
	}
	if _, ok := draft[id]; !ok {
		return nil, false, fmt.Errorf("agent not found: %s", id)
	}
	// Preserve a stable admission input; session construction replays the plugin definition.
	return &store.Agent{ID: id}, true, nil
}

func (s *Server) captureRunDraft(candidate *store.SessionRun, draft sessionDraft, directory string) error {
	instructions, sources, err := s.resolveInstructions(&draft.Agent, directory)
	if err != nil {
		return err
	}
	if catalog := s.skillCatalog(&draft.Agent, draft.Skills); catalog != "" {
		instructions += "\n\n" + catalog
	}
	candidate.PreparedAgent = &draft.Agent
	candidate.EffectiveInstructions, candidate.InstructionSources, candidate.Skills = instructions, sources, draft.Skills
	return nil
}

func transformSkills(ctx context.Context, activation *pluginhost.Activation, skills []skill.Skill) ([]skill.Skill, error) {
	input := protocol.Skills{}
	for _, entry := range skills {
		input[entry.ID] = entry
	}
	var output protocol.Skills
	if err := activation.Transform(ctx, "skills", input, &output); err != nil {
		return nil, err
	}
	result := make([]skill.Skill, 0, len(output))
	for id, entry := range output {
		if id == "" || entry.ID != id || entry.Name == "" {
			return nil, fmt.Errorf("invalid transformed skill %q", id)
		}
		entry.SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(entry.Content)))
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (s *Server) transformReferences(ctx context.Context, activation *pluginhost.Activation) ([]orientation.Reference, error) {
	input := protocol.References{}
	for _, ref := range s.references {
		input[ref.Name] = ref
	}
	var output protocol.References
	if err := activation.Transform(ctx, "references", input, &output); err != nil {
		return nil, err
	}
	result := make([]orientation.Reference, 0, len(output))
	for name, ref := range output {
		if name == "" || ref.Name != name || !filepath.IsAbs(ref.Path) {
			return nil, fmt.Errorf("invalid transformed reference %q", name)
		}
		result = append(result, ref)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
