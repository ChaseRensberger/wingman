package main

import "context"

type Session struct {
	inferenceProvider InferenceProvider
	history           []any
}

func CreateSession(inferenceProvider InferenceProvider) *Session {
	return &Session{
		inferenceProvider: inferenceProvider,
		history:           []any{},
	}
}

func (s *Session) RunInference(ctx context.Context, input any) (any, error) {
	result, err := s.inferenceProvider.RunInference(ctx, input)
	if err != nil {
		return nil, err
	}

	s.history = append(s.history, input, result)

	return result, nil
}
