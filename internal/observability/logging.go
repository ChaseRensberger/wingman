package observability

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// NewLogger builds Wingman's process logger. JSON is the default because
// the server is primarily consumed as infrastructure.
func NewLogger(out io.Writer, format, level string) (*slog.Logger, error) {
	if out == nil {
		out = os.Stderr
	}

	var lvl slog.Level
	switch strings.ToLower(level) {
	case "", "info":
		lvl = slog.LevelInfo
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log level %q", level)
	}

	opts := &slog.HandlerOptions{Level: lvl}
	switch strings.ToLower(format) {
	case "", "json":
		return slog.New(slog.NewJSONHandler(out, opts)), nil
	case "text":
		return slog.New(slog.NewTextHandler(out, opts)), nil
	default:
		return nil, fmt.Errorf("invalid log format %q", format)
	}
}

func NewBufferedLogger(out io.Writer, format, level string, buffer *LogBuffer) (*slog.Logger, error) {
	if buffer != nil {
		out = io.MultiWriter(out, buffer)
	}
	return NewLogger(out, format, level)
}

// ConfigureDefault installs the process-wide logger used by packages that do
// not receive an explicit logger.
func ConfigureDefault(format, level string) (*slog.Logger, error) {
	return ConfigureDefaultWithBuffer(format, level, nil)
}

func ConfigureDefaultWithBuffer(format, level string, buffer *LogBuffer) (*slog.Logger, error) {
	logger, err := NewBufferedLogger(os.Stderr, format, level, buffer)
	if err != nil {
		return nil, err
	}
	slog.SetDefault(logger)
	return logger, nil
}
