package agent

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

type MessageType string

const (
	MsgPrompt     MessageType = "prompt"
	MsgResponse   MessageType = "response"
	MsgToolCall   MessageType = "tool_call"
	MsgToolResult MessageType = "tool_result"
	MsgStatus     MessageType = "status"
	MsgError      MessageType = "error"
	MsgShutdown   MessageType = "shutdown"
)

type InboxMessage struct {
	ID        string
	From      string
	To        string
	Type      MessageType
	Payload   any
	Timestamp time.Time
}

func NewInboxMessage(from, to string, msgType MessageType, payload any) InboxMessage {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)

	return InboxMessage{
		ID:        id.String(),
		From:      from,
		To:        to,
		Type:      msgType,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}

type Inbox struct {
	messages chan InboxMessage
	done     chan struct{}
	size     int
}

func NewInbox(size int) *Inbox {
	return &Inbox{
		messages: make(chan InboxMessage, size),
		done:     make(chan struct{}),
		size:     size,
	}
}

func (in *Inbox) Send(msg InboxMessage) error {
	select {
	case in.messages <- msg:
		return nil
	case <-in.done:
		return fmt.Errorf("inbox closed")
	default:
		return fmt.Errorf("inbox full")
	}
}

func (in *Inbox) Receive(ctx context.Context) (InboxMessage, error) {
	select {
	case msg := <-in.messages:
		return msg, nil
	case <-ctx.Done():
		return InboxMessage{}, ctx.Err()
	case <-in.done:
		return InboxMessage{}, fmt.Errorf("inbox closed")
	}
}

func (in *Inbox) ReceiveWithTimeout(timeout time.Duration) (InboxMessage, error) {
	select {
	case msg := <-in.messages:
		return msg, nil
	case <-time.After(timeout):
		return InboxMessage{}, fmt.Errorf("timeout after %v", timeout)
	case <-in.done:
		return InboxMessage{}, fmt.Errorf("inbox closed")
	}
}

func (in *Inbox) Close() {
	close(in.done)
}
