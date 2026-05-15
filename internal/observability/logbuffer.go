package observability

import (
	"encoding/json"
	"strings"
	"sync"
)

// LogEntry is a single process-local log line prepared for API/UI consumers.
type LogEntry struct {
	Raw   string         `json:"raw"`
	Time  string         `json:"time,omitempty"`
	Level string         `json:"level,omitempty"`
	Msg   string         `json:"msg,omitempty"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// LogBuffer is a bounded, process-local sink for recent log lines.
type LogBuffer struct {
	mu      sync.Mutex
	limit   int
	entries []LogEntry
	pending string
}

func NewLogBuffer(limit int) *LogBuffer {
	if limit <= 0 {
		limit = 500
	}
	return &LogBuffer{limit: limit}
}

func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.pending += string(p)
	for {
		idx := strings.IndexByte(b.pending, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSpace(b.pending[:idx])
		b.pending = b.pending[idx+1:]
		if line != "" {
			b.appendLocked(parseLogEntry(line))
		}
	}
	return len(p), nil
}

func (b *LogBuffer) Entries() []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	entries := make([]LogEntry, len(b.entries))
	copy(entries, b.entries)
	return entries
}

func (b *LogBuffer) appendLocked(entry LogEntry) {
	if len(b.entries) == b.limit {
		copy(b.entries, b.entries[1:])
		b.entries[len(b.entries)-1] = entry
		return
	}
	b.entries = append(b.entries, entry)
}

func parseLogEntry(line string) LogEntry {
	entry := LogEntry{Raw: line}

	var fields map[string]any
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		return entry
	}

	if value, ok := fields["time"].(string); ok {
		entry.Time = value
		delete(fields, "time")
	}
	if value, ok := fields["level"].(string); ok {
		entry.Level = value
		delete(fields, "level")
	}
	if value, ok := fields["msg"].(string); ok {
		entry.Msg = value
		delete(fields, "msg")
	}
	if len(fields) > 0 {
		entry.Attrs = fields
	}

	return entry
}
