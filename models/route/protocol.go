// Package route composes model protocols, endpoints, authentication, and transports.
package route

import "github.com/chaserensberger/wingman/models"

// Protocol converts common requests and native response frames without deployment knowledge.
type Protocol interface {
	ID() string
	Body(models.ModelInfo, models.Request) (map[string]any, error)
	NewParser(models.ModelInfo) Parser
	LoweredOptions(models.Request) models.LoweredOptions
}

// Parser owns one response's native state and emits normalized stream parts.
type Parser interface {
	Step(Frame) ([]models.StreamPart, error)
	End() ([]models.StreamPart, error)
	Message() *models.Message
}
