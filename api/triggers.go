package api

import (
	"time"

	"github.com/chaserensberger/wingman/trigger"
)

// UpdateTriggerRequest replaces a trigger definition at the expected version.
type UpdateTriggerRequest struct {
	trigger.Config
	ExpectedVersion int64 `json:"expected_version"`
}

// TriggerStateRequest changes whether a trigger can fire automatically.
type TriggerStateRequest struct {
	Enabled         bool  `json:"enabled"`
	ExpectedVersion int64 `json:"expected_version"`
}

// FireTriggerRequest identifies one manual execution, including retries.
type FireTriggerRequest struct {
	RequestID string `json:"request_id"`
}

// TriggerPreview lists the next scheduled times for a source.
type TriggerPreview struct {
	Times []time.Time `json:"times"`
}
