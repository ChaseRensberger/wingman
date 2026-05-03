package storage

import (
	"fmt"
	"strings"

	"github.com/segmentio/ksuid"
)

// ID prefixes. Every persisted entity carries a stable prefix tag so that
// IDs are self-describing in logs, URLs, and error messages. The prefix
// is part of the primary key; we store and compare it verbatim.
//
// Prefixes are short, lowercase, and underscore-separated from the body.
// Adding a new entity type? Add the constant here, not inline.
const (
	PrefixAgent   = "agt_"
	PrefixSession = "ses_"
	PrefixMessage = "msg_"
	PrefixPart    = "prt_"
	PrefixToolUse = "tlu_"
)

// NewID returns a freshly minted KSUID prefixed with prefix. The body is
// 27 base62 characters; the full string is len(prefix)+27.
//
// KSUID over ULID: smaller (27 vs 26 chars), 32-bit timestamp resolution
// (good through 2150), and lex-sortable by time without requiring
// monotonic entropy state.
func NewID(prefix string) string {
	return prefix + ksuid.New().String()
}

// ParseID splits a prefixed ID into (prefix, body). Returns an error if
// the input does not match a known prefix. Use this when you need to
// validate that an ID belongs to a specific entity type at an API
// boundary.
//
// Unknown prefixes are rejected to catch accidentally-typed IDs early
// (a session ID where an agent ID was expected, etc.).
func ParseID(id string) (prefix, body string, err error) {
	for _, p := range []string{PrefixAgent, PrefixSession, PrefixMessage, PrefixPart, PrefixToolUse} {
		if strings.HasPrefix(id, p) {
			return p, id[len(p):], nil
		}
	}
	return "", "", fmt.Errorf("storage.ParseID: unknown prefix in %q", id)
}
