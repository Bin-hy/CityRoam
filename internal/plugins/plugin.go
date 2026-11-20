package plugins

import (
	"trippilot/internal/agent"
)

// Status is the externally visible state of a plugin. DisabledReason must be
// safe to expose through /api/meta: never include keys or private URLs.
type Status struct {
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	DisabledReason string `json:"disabled_reason,omitempty"`
}

// Plugin groups the tools of one integration behind a single switch.
type Plugin interface {
	Name() string
	Status() Status
	Tools() []agent.ResearchTool
}

// Registry holds plugins in configuration order so status output is stable.
type Registry struct {
	plugins []Plugin
}

func NewRegistry(plugins ...Plugin) *Registry {
	registry := &Registry{}
	for _, plugin := range plugins {
		if plugin == nil {
			continue
		}
		registry.plugins = append(registry.plugins, plugin)
	}
	return registry
}

// ActiveTools returns a fresh slice; callers may sort or filter it without
// mutating the registry.
func (r *Registry) ActiveTools() []agent.ResearchTool {
	if r == nil {
		return nil
	}
	tools := make([]agent.ResearchTool, 0)
	for _, plugin := range r.plugins {
		if !plugin.Status().Enabled {
			continue
		}
		for _, tool := range plugin.Tools() {
			if tool != nil {
				tools = append(tools, tool)
			}
		}
	}
	return tools
}

func (r *Registry) Statuses() []Status {
	if r == nil {
		return nil
	}
	statuses := make([]Status, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		status := plugin.Status()
		if status.Name == "" {
			status.Name = plugin.Name()
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func (r *Registry) Plugins() []Plugin {
	if r == nil {
		return nil
	}
	return append([]Plugin(nil), r.plugins...)
}
