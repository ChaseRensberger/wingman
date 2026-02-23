// Package models re-exports the canonical types from core. New code should
// import core directly. This package is kept for backward compatibility during
// migration.
package models

import "github.com/chaserensberger/wingman/core"

// Role aliases
type WingmanRole = core.Role

const (
	RoleUser      = core.RoleUser
	RoleAssistant = core.RoleAssistant
)

// ContentType aliases
type WingmanContentType = core.ContentType

const (
	ContentTypeText       = core.ContentTypeText
	ContentTypeToolUse    = core.ContentTypeToolUse
	ContentTypeToolResult = core.ContentTypeToolResult
)

// Type aliases
type WingmanContentBlock = core.ContentBlock
type WingmanMessage = core.Message
type WingmanToolDefinition = core.ToolDefinition
type WingmanToolInputSchema = core.ToolInputSchema
type WingmanToolProperty = core.ToolProperty
type WingmanUsage = core.Usage
type WingmanInferenceRequest = core.InferenceRequest
type WingmanInferenceResponse = core.InferenceResponse

// Constructor aliases
var NewUserMessage = core.NewUserMessage
var NewAssistantMessage = core.NewAssistantMessage
var NewToolResultMessage = core.NewToolResultMessage
