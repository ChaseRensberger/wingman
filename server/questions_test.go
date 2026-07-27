package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/tool"
)

func TestQuestionBrokerListEmptyIsJSONArray(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(newQuestionBroker().list("ses_1"))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(data) != "[]" {
		t.Fatalf("empty list JSON = %s", data)
	}
}

func TestQuestionBrokerReplyRemovesPendingRequest(t *testing.T) {
	t.Parallel()
	broker := newQuestionBroker()
	published := make(chan *pendingQuestion, 1)
	result := make(chan [][]string, 1)
	errs := make(chan error, 1)
	go func() {
		answers, err := broker.ask(context.Background(), "ses_1", "call_1", []tool.Question{{Question: "Continue?"}}, func(_ string, value any) {
			published <- value.(*pendingQuestion)
		})
		result <- answers
		errs <- err
	}()

	request := <-published
	if request.CallID != "call_1" {
		t.Fatalf("call ID = %q", request.CallID)
	}
	if err := broker.answer("ses_1", request.ID, [][]string{{"Yes"}}, false); err != nil {
		t.Fatalf("answer() error = %v", err)
	}
	if pending := broker.list("ses_1"); len(pending) != 0 {
		t.Fatalf("pending = %#v", pending)
	}
	if err := broker.answer("ses_1", request.ID, nil, false); err == nil {
		t.Fatal("duplicate answer error = nil")
	}
	if err := <-errs; err != nil {
		t.Fatalf("ask() error = %v", err)
	}
	if answers := <-result; len(answers) != 1 || len(answers[0]) != 1 || answers[0][0] != "Yes" {
		t.Fatalf("answers = %#v", answers)
	}
}

func TestQuestionBrokerDismissalRejectsAsk(t *testing.T) {
	t.Parallel()
	broker := newQuestionBroker()
	published := make(chan *pendingQuestion, 1)
	errs := make(chan error, 1)
	go func() {
		_, err := broker.ask(context.Background(), "ses_1", "call_1", []tool.Question{{Question: "Continue?"}}, func(_ string, value any) {
			published <- value.(*pendingQuestion)
		})
		errs <- err
	}()

	request := <-published
	if err := broker.answer("ses_1", request.ID, nil, true); err != nil {
		t.Fatalf("dismiss() error = %v", err)
	}
	if err := <-errs; !errors.Is(err, errQuestionDismissed) {
		t.Fatalf("ask() error = %v", err)
	}
}
