package protocol

import (
	"encoding/json"
	provider "github.com/chaserensberger/wingman/models/providers"
)

// AuthMethod declares a client-discoverable integration sign-in method.
type AuthMethod struct {
	ID          string `json:"id"`
	Integration string `json:"integration"`
	Label       string `json:"label"`
	Kind        string `json:"kind" enum:"key,oauth,command"`
}

// AuthCall supplies private attempt state to the executable that owns it.
type AuthCall struct {
	Identity  Identity        `json:"context"`
	Method    string          `json:"method"`
	Operation string          `json:"operation"`
	Input     json.RawMessage `json:"input,omitempty"`
	State     json.RawMessage `json:"state,omitempty"`
}

// AuthResult separates public instructions from private state and credentials.
type AuthResult struct {
	Status       string               `json:"status" enum:"pending,connected,cancelled"`
	URL          string               `json:"url,omitempty"`
	Instructions string               `json:"instructions,omitempty"`
	State        json.RawMessage      `json:"state,omitempty"`
	Credential   *provider.Credential `json:"credential,omitempty"`
}
