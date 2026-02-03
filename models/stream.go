package models

type StreamEventType string

const (
	EventMessageStart      StreamEventType = "message_start"
	EventContentBlockStart StreamEventType = "content_block_start"
	EventTextDelta         StreamEventType = "text_delta"
	EventInputJSONDelta    StreamEventType = "input_json_delta"
	EventContentBlockStop  StreamEventType = "content_block_stop"
	EventMessageDelta      StreamEventType = "message_delta"
	EventMessageStop       StreamEventType = "message_stop"
	EventPing              StreamEventType = "ping"
	EventError             StreamEventType = "error"
)

type StreamEvent struct {
	Type         StreamEventType
	Text         string
	InputJSON    string
	Index        int
	ContentBlock *StreamContentBlock
	StopReason   string
	Usage        *WingmanUsage
	Error        error
}

type StreamContentBlock struct {
	Type  string
	ID    string
	Name  string
	Text  string
	Input map[string]any
}
