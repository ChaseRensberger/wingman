// Stream event types re-exported from core for backward compatibility.
package models

import "github.com/chaserensberger/wingman/core"

type StreamEventType = core.StreamEventType
type StreamEvent = core.StreamEvent
type StreamContentBlock = core.StreamContentBlock

const (
	EventMessageStart      = core.EventMessageStart
	EventContentBlockStart = core.EventContentBlockStart
	EventTextDelta         = core.EventTextDelta
	EventInputJSONDelta    = core.EventInputJSONDelta
	EventContentBlockStop  = core.EventContentBlockStop
	EventMessageDelta      = core.EventMessageDelta
	EventMessageStop       = core.EventMessageStop
	EventPing              = core.EventPing
	EventError             = core.EventError
)
