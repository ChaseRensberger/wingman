// Package provider defines the Provider and Stream interfaces and re-exports
// them from core. It also contains the provider registry with factory support.
package provider

import "github.com/chaserensberger/wingman/core"

// Provider is the interface every LLM backend must implement.
// Re-exported from core for convenience.
type Provider = core.Provider

// Stream is the interface returned by Provider.StreamInference.
// Re-exported from core for convenience.
type Stream = core.Stream
