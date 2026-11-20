package agent

import (
	"encoding/json"
)

// ToolPromptInfo is the prompt-safe description of an enabled research tool.
type ToolPromptInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

// DisabledPlugin explains why a plugin is off without exposing credentials or
// internal endpoints.
type DisabledPlugin struct {
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

// ResearchPromptData feeds prompts/research.tmpl.
type ResearchPromptData struct {
	Input       string
	Constraints json.RawMessage
	Candidates  json.RawMessage
	Tools       []ToolPromptInfo
	Disabled    []DisabledPlugin
	Today       string
	Nights      int
}
