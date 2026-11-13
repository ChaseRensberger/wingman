package plugin

import (
	"context"
	"github.com/chaserensberger/wingman/plugin/protocol"
)

// Authentication implements start, status, complete, and cancel for one sign-in method.
// Private state and credentials returned by Handle never appear in client responses.
type Authentication struct {
	protocol.AuthMethod
	Handle func(context.Context, protocol.AuthCall) (protocol.AuthResult, error)
}
