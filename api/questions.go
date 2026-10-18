package api

import "github.com/chaserensberger/wingman/question"

// CreateQuestionRequest creates a client-answered question independently of a tool.
type CreateQuestionRequest struct {
	ID        string            `json:"id,omitempty"`
	Questions []question.Prompt `json:"questions"`
}

// QuestionReplyRequest answers or dismisses an existing question.
type QuestionReplyRequest struct {
	Status  string     `json:"status" enum:"answered,dismissed"`
	Answers [][]string `json:"answers,omitempty"`
}
