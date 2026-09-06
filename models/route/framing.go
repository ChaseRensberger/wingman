package route

import (
	"bufio"
	"context"
	"io"
	"iter"
	"strings"
)

// Frame is one transport event, before protocol decoding.
type Frame struct {
	Event string
	Data  string
}

// Framing splits response bytes into opaque protocol frames.
type Framing func(context.Context, io.Reader) iter.Seq2[Frame, error]

// SSE decodes server-sent events without interpreting their data.
func SSE(ctx context.Context, reader io.Reader) iter.Seq2[Frame, error] {
	return func(yield func(Frame, error) bool) {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var event string
		var data []string
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				yield(Frame{}, err)
				return
			}
			line := scanner.Text()
			if line == "" {
				if payload := strings.Join(data, "\n"); payload != "" && !yield(Frame{Event: event, Data: payload}, nil) {
					return
				}
				event, data = "", nil
				continue
			}
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "event":
				event = value
			case "data":
				data = append(data, value)
			}
		}
		if err := ctx.Err(); err != nil {
			yield(Frame{}, err)
			return
		}
		if err := scanner.Err(); err != nil {
			yield(Frame{}, err)
			return
		}
		if payload := strings.Join(data, "\n"); payload != "" {
			yield(Frame{Event: event, Data: payload}, nil)
		}
	}
}
