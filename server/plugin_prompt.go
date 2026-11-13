package server

import "context"

type admissionKey struct{ session, request string }
type admissionGate struct {
	token chan struct{}
	refs  int
}

func (s *Server) lockAdmission(ctx context.Context, session, request string) (func(), error) {
	if request == "" {
		return func() {}, nil
	}
	key := admissionKey{session, request}
	s.admissionMu.Lock()
	if s.admissions == nil {
		s.admissions = map[admissionKey]*admissionGate{}
	}
	gate := s.admissions[key]
	if gate == nil {
		gate = &admissionGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		s.admissions[key] = gate
	}
	gate.refs++
	s.admissionMu.Unlock()
	release := func() {
		s.admissionMu.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(s.admissions, key)
		}
		s.admissionMu.Unlock()
	}
	select {
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	case <-gate.token:
		return func() { gate.token <- struct{}{}; release() }, nil
	}
}
