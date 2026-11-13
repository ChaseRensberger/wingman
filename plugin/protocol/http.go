package protocol

import (
	"github.com/chaserensberger/wingman/models"
	"net/http"
)

// HTTPExchange describes one native model exchange; its body is accessed through owned RPC reads.
type HTTPExchange struct {
	Identity      Identity        `json:"context"`
	Model         models.ModelRef `json:"model"`
	Method        string          `json:"method"`
	URL           string          `json:"url"`
	Headers       http.Header     `json:"headers"`
	Status        int             `json:"status,omitempty"`
	ContentLength int64           `json:"content_length"`
	HasBody       bool            `json:"has_body"`
	BodyID        string          `json:"body_id,omitempty"`
}

// BodyChunk transfers at most 32 KiB and an explicit end-of-stream marker.
type BodyChunk struct {
	Data []byte `json:"data"`
	EOF  bool   `json:"eof"`
}

// BodyRead identifies a plugin-owned replacement body.
type BodyRead struct {
	ID string `json:"id"`
}
