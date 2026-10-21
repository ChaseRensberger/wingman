package store

const (
	EventSessionAgentSwitched = "session.agent.switched"
	EventSessionModelSwitched = "session.model.switched"
)

// SessionSwitch describes a change applied at the start of a turn.
type SessionSwitch struct {
	Type string
	Data map[string]any
}

// SessionRunSwitches compares a new turn with the previous started turn.
func SessionRunSwitches(current SessionRun, previous *SessionRun) []SessionSwitch {
	if previous == nil || !current.StartedAt.IsZero() {
		return nil
	}
	var switches []SessionSwitch
	if current.Agent.ID != previous.Agent.ID {
		switches = append(switches, SessionSwitch{Type: EventSessionAgentSwitched, Data: map[string]any{
			"agent": current.Agent.ID, "name": current.Agent.Name,
			"previous": previous.Agent.ID, "previous_name": previous.Agent.Name,
		}})
	}
	if current.Agent.ModelRef != previous.Agent.ModelRef {
		switches = append(switches, SessionSwitch{Type: EventSessionModelSwitched, Data: map[string]any{
			"model_ref": current.Agent.ModelRef, "previous": previous.Agent.ModelRef,
		}})
	}
	return switches
}
