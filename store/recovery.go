package store

import "bytes"

// MaxRunRecoveryAttempts bounds automatic restarts of one run.
const MaxRunRecoveryAttempts = 3

// IsToolUseRecovery identifies a safe reset of an interrupted invocation.
func IsToolUseRecovery(previous, next ToolUse) bool {
	if previous.Status != ToolUseStatusInterrupted || (!previous.StartedAt.IsZero() && !previous.ReplaySafe) {
		return false
	}
	if !bytes.Equal(previous.InputJSON, next.InputJSON) {
		return false
	}
	status := ToolUseStatusAuthorized
	if previous.AuthorizedAt.IsZero() {
		status = ToolUseStatusProposed
	}
	return next.Status == status && next.StartedAt.Equal(previous.StartedAt) && next.CompletedAt.IsZero() && next.ErrorType == "" && next.ErrorMessage == ""
}
