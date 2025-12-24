package agent

import (
	"fmt"
	"time"
)

type MessageContent struct {
	From    string
	Payload any
}

type Inbox struct {
	messages chan MessageContent
	size     int
}

func NewInbox(size int) *Inbox {
	return &Inbox{
		messages: make(chan MessageContent, size),
		size:     size,
	}
}

func (in *Inbox) Send(content MessageContent) error {
	select {
	case in.messages <- content:
		return nil
	default:
		return fmt.Errorf("inbox full")
	}
}

func (in *Inbox) Receive(timeout time.Duration) (MessageContent, error) {
	select {
	case msg := <-in.messages:
		return msg, nil
	case <-time.After(timeout):
		return MessageContent{}, fmt.Errorf("timeout")
	}
}
