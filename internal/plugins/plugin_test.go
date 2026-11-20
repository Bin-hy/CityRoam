package plugins

import (
	"testing"

	"trippilot/internal/agent"
)

func TestRegistry(t *testing.T) {
	hotel := &stubTool{name: "search_hotels"}
	guide := &stubTool{name: "search_travel_notes"}
	enabled := &stubPlugin{
		name:   "hotel",
		status: Status{Name: "hotel", Enabled: true},
		tools:  []agent.ResearchTool{hotel},
	}
	disabled := &stubPlugin{
		name:   "guide",
		status: Status{Name: "guide", Enabled: false, DisabledReason: "GUIDE_SEARCH_API_KEY is empty"},
		tools:  []agent.ResearchTool{guide},
	}
	registry := NewRegistry(nil, enabled, disabled)

	active := registry.ActiveTools()
	if len(active) != 1 || active[0].Name() != "search_hotels" {
		t.Fatalf("active = %+v", active)
	}
	active[0] = guide
	if registry.ActiveTools()[0].Name() != "search_hotels" {
		t.Fatal("ActiveTools must return an independent slice")
	}

	statuses := registry.Statuses()
	if len(statuses) != 2 {
		t.Fatalf("statuses = %+v", statuses)
	}
	if statuses[0].Name != "hotel" || !statuses[0].Enabled {
		t.Fatalf("first status = %+v", statuses[0])
	}
	if statuses[1].Enabled || statuses[1].DisabledReason == "" {
		t.Fatalf("second status = %+v", statuses[1])
	}
	if len(registry.Plugins()) != 2 {
		t.Fatalf("plugins = %+v", registry.Plugins())
	}
}

func TestRegistryStatusNameFallback(t *testing.T) {
	plugin := &stubPlugin{name: "hotel", status: Status{Enabled: true}}
	registry := NewRegistry(plugin)
	statuses := registry.Statuses()
	if len(statuses) != 1 || statuses[0].Name != "hotel" {
		t.Fatalf("statuses = %+v", statuses)
	}
}
