package server

import "fmt"

func (s *Server) validateAgentPlugins(names []string) error {
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			return fmt.Errorf("agent plugin %q is configured more than once", name)
		}
		if s.agentPluginFactories[name] == nil {
			return fmt.Errorf("agent plugin %q is unavailable", name)
		}
		seen[name] = true
	}
	return nil
}
