package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"trippilot/internal/tools"
)

// Adapter exposes one allowlisted MCP tool as a native tools.Tool so the agent
// can treat native and MCP-provided capabilities uniformly.
type Adapter struct {
	hub    *Hub
	server string
	tool   string
	name   string
	schema json.RawMessage
}

var _ tools.Tool = (*Adapter)(nil)

// NewAdapter discovers the tool schema once. The alias falls back to the MCP
// tool name when empty.
func NewAdapter(ctx context.Context, hub *Hub, server, tool, alias string) (*Adapter, error) {
	if hub == nil {
		return nil, errors.New("mcp: nil hub")
	}
	if alias == "" {
		alias = tool
	}
	schema, err := hub.ToolSchema(ctx, server, tool)
	if err != nil {
		return nil, err
	}
	return &Adapter{hub: hub, server: server, tool: tool, name: alias, schema: schema}, nil
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) InputSchema() json.RawMessage { return a.schema }

func (a *Adapter) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	args := map[string]any{}
	if len(input) > 0 && string(input) != "null" {
		if err := json.Unmarshal(input, &args); err != nil {
			return nil, fmt.Errorf("mcp: %s arguments must be a JSON object: %w", a.name, err)
		}
	}

	result, err := a.hub.Call(ctx, a.server, a.tool, args)
	if err != nil {
		return nil, err
	}
	return result.JSON()
}

// JSON flattens an MCP result into the JSON payload the agent consumes.
// Structured content wins; otherwise a single JSON text part is passed through
// and plain text parts are wrapped in {"content": [...]}.
func (r *Result) JSON() (json.RawMessage, error) {
	if len(r.Structured) > 0 && string(r.Structured) != "null" {
		return r.Structured, nil
	}

	texts := make([]string, 0, len(r.Content))
	for _, content := range r.Content {
		if text, ok := content.(*sdk.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	switch len(texts) {
	case 1:
		trimmed := strings.TrimSpace(texts[0])
		if json.Valid([]byte(trimmed)) {
			return json.RawMessage(trimmed), nil
		}
		if r.IsError {
			return nil, fmt.Errorf("mcp tool error: %s", trimmed)
		}
		return marshalString(trimmed)
	case 0:
		if r.IsError {
			return nil, errors.New("mcp tool returned an error without content")
		}
		return json.RawMessage(`{}`), nil
	default:
		if r.IsError {
			return nil, fmt.Errorf("mcp tool error: %s", strings.Join(texts, "\n"))
		}
		return json.Marshal(map[string][]string{"content": texts})
	}
}

func marshalString(text string) (json.RawMessage, error) {
	encoded, err := json.Marshal(text)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}
