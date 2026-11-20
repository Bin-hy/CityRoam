package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"trippilot/internal/domain"
	"trippilot/internal/skills"
)

func skillFixture(version, body string) skills.Skill {
	return skills.Skill{
		Name:        "research-playbook",
		Version:     version,
		Description: "研究清单",
		SHA256:      strings.Repeat("a", 64),
		Body:        body,
	}
}

func TestResearchLoadsPinnedSkill(t *testing.T) {
	tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return guideToolOutput(t), nil
	}}
	llm := &fakeToolLLM{responses: []ToolCompletion{
		{ToolCalls: []ToolCall{toolCall("c1", researchSkillTool, `{"name":"research-playbook"}`)}},
		{Content: briefJSON(t, `{"status":"ok"}`)},
	}}
	events := &eventRecorder{}
	nodes := researchTestNodes(nil, events, WithToolLLM(llm), WithResearchTools(tool))

	in := researchTestInput()
	in.Skills = []skills.Skill{skillFixture("1.0.0", "先核对闭馆日，再排路线。")}
	out, err := nodes.Research(context.Background(), in)
	if err != nil {
		t.Fatalf("research: %v", err)
	}
	if out.Brief.Status != "ok" {
		t.Fatalf("status = %s", out.Brief.Status)
	}

	first := llm.requestAt(t, 0)
	if len(first.Messages) == 0 || !strings.Contains(first.Messages[0].Content, "research-playbook@1.0.0") || !strings.Contains(first.Messages[0].Content, "研究清单") {
		t.Fatalf("user prompt missing skill catalog: %+v", first.Messages)
	}
	names := make([]string, 0, len(first.Tools))
	for _, definition := range first.Tools {
		names = append(names, definition.Name)
	}
	if !containsString(names, researchSkillTool) || !containsString(names, researchGuideTool) {
		t.Fatalf("tools = %v", names)
	}

	second := llm.requestAt(t, 1)
	if len(second.Messages) < 3 {
		t.Fatalf("messages = %+v", second.Messages)
	}
	content := second.Messages[2].Content
	if !strings.Contains(content, `<skill name="research-playbook" version="1.0.0"`) {
		t.Fatalf("tool content = %s", content)
	}
	if !strings.Contains(content, "先核对闭馆日") {
		t.Fatalf("tool content missing body: %s", content)
	}
	if strings.Contains(content, "<external_data>") {
		t.Fatalf("trusted skill body was wrapped as external data: %s", content)
	}
	if !containsString(events.types(), "research.skill_loaded") {
		t.Fatalf("events = %v", events.types())
	}
}

func TestResearchSkillsWithoutToolsDoNotStartLoop(t *testing.T) {
	llm := &fakeToolLLM{responses: []ToolCompletion{{Content: briefJSON(t, `{"status":"ok"}`)}}}
	events := &eventRecorder{}
	nodes := researchTestNodes(nil, events, WithToolLLM(llm))

	in := researchTestInput()
	in.Skills = []skills.Skill{skillFixture("1.0.0", "先核对闭馆日，再排路线。")}
	out, err := nodes.Research(context.Background(), in)
	if err != nil {
		t.Fatalf("research: %v", err)
	}
	if out.Brief.Status != domain.ResearchSkipped {
		t.Fatalf("status = %s", out.Brief.Status)
	}
	if llm.requestCount() != 0 {
		t.Fatalf("skills alone started a tool loop: %d requests", llm.requestCount())
	}
	if containsString(events.types(), "research.started") {
		t.Fatalf("events = %v", events.types())
	}
}

func TestResearchUnknownSkillDoesNotDegrade(t *testing.T) {
	tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return guideToolOutput(t), nil
	}}
	llm := &fakeToolLLM{responses: []ToolCompletion{
		{ToolCalls: []ToolCall{toolCall("c1", researchSkillTool, `{"name":"missing"}`)}},
		{Content: briefJSON(t, `{"status":"ok"}`)},
	}}
	nodes := researchTestNodes(nil, &eventRecorder{}, WithToolLLM(llm), WithResearchTools(tool))

	in := researchTestInput()
	in.Skills = []skills.Skill{skillFixture("1.0.0", "body")}
	out, err := nodes.Research(context.Background(), in)
	if err != nil {
		t.Fatalf("research: %v", err)
	}
	if out.Brief.Status != "ok" {
		t.Fatalf("unknown skill degraded the brief: %s", out.Brief.Status)
	}
	content := llm.requestAt(t, 1).Messages[2].Content
	if !strings.Contains(content, "unknown_skill") {
		t.Fatalf("tool content = %s", content)
	}
}

func TestAdmitPinsSkillsForRun(t *testing.T) {
	ctx := context.Background()
	v1 := skills.NewRegistry(skillFixture("1.0.0", "OLD RULES"))
	first := &Graph{nodes: NewNodes(nil, &eventRecorder{}, WithSkillRegistry(v1), WithNodeLogger(discardLogger()))}

	state := &TripState{RunID: "run-1", Input: "北京一天"}
	out, err := first.admit(ctx, state)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if !out.SkillsPinned || len(out.SkillPins) != 1 || out.SkillPins[0].Version != "1.0.0" {
		t.Fatalf("pinned state = %+v", out)
	}
	if len(out.Skills) != 0 {
		t.Fatalf("new run stored skill bodies: %+v", out.Skills)
	}

	encoded, err := out.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(string(encoded), "OLD RULES") || !strings.Contains(string(encoded), `"skill_pins"`) {
		t.Fatalf("encoded state = %s", encoded)
	}
	restored, err := DecodeTripState(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	v2 := skills.NewRegistry(skillFixture("1.1.0", "NEW RULES"))
	second := &Graph{nodes: NewNodes(nil, &eventRecorder{}, WithSkillRegistry(v2), WithNodeLogger(discardLogger()))}
	resumed, err := second.admit(ctx, restored)
	if err != nil {
		t.Fatalf("admit resumed: %v", err)
	}
	if resumed.SkillPins[0].Version != "1.0.0" {
		t.Fatalf("resumed run picked up the upgrade: %+v", resumed.SkillPins)
	}

	fresh := &TripState{RunID: "run-2", Input: "北京一天"}
	next, err := second.admit(ctx, fresh)
	if err != nil {
		t.Fatalf("admit fresh: %v", err)
	}
	if next.SkillPins[0].Version != "1.1.0" {
		t.Fatalf("new run did not use the new pin: %+v", next.SkillPins)
	}
}

func TestAdmitKeepsLegacySkillBodies(t *testing.T) {
	provider := &countingProvider{}
	nodes := NewNodes(nil, &eventRecorder{}, WithSkillProvider(provider), WithNodeLogger(discardLogger()))
	graph := &Graph{nodes: nodes}
	state := &TripState{
		RunID:        "run-legacy",
		Input:        "北京一天",
		SkillsPinned: true,
		Skills:       []skills.Skill{skillFixture("1.0.0", "LEGACY BODY")},
	}
	out, err := graph.admit(context.Background(), state)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if len(out.Skills) != 1 || out.Skills[0].Body != "LEGACY BODY" {
		t.Fatalf("legacy bodies were dropped: %+v", out.Skills)
	}
	if provider.listCalls != 0 {
		t.Fatalf("pinned run re-read the catalog %d time(s)", provider.listCalls)
	}
}

func TestAdmitReportsSkillCatalogFailure(t *testing.T) {
	nodes := NewNodes(nil, &eventRecorder{}, WithSkillProvider(failingProvider{}), WithNodeLogger(discardLogger()))
	graph := &Graph{nodes: nodes}
	_, err := graph.admit(context.Background(), &TripState{RunID: "run-1", Input: "北京一天"})
	var nodeErr *NodeError
	if !errors.As(err, &nodeErr) || nodeErr.Code != "skills_unavailable" {
		t.Fatalf("err = %v", err)
	}
}

type countingProvider struct {
	listCalls int
}

func (p *countingProvider) List(context.Context) ([]skills.CatalogEntry, error) {
	p.listCalls++
	return nil, nil
}

func (p *countingProvider) Resolve(context.Context, skills.CatalogEntry) (skills.Skill, error) {
	return skills.Skill{}, errors.New("not used")
}

type failingProvider struct{}

func (failingProvider) List(context.Context) ([]skills.CatalogEntry, error) {
	return nil, errors.New("scan failed")
}

func (failingProvider) Resolve(context.Context, skills.CatalogEntry) (skills.Skill, error) {
	return skills.Skill{}, errors.New("not used")
}

func TestResearchResolvesPinnedSkillThroughProvider(t *testing.T) {
	tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return guideToolOutput(t), nil
	}}
	llm := &fakeToolLLM{responses: []ToolCompletion{
		{ToolCalls: []ToolCall{toolCall("c1", researchSkillTool, `{"name":"research-playbook"}`)}},
		{Content: briefJSON(t, `{"status":"ok"}`)},
	}}
	events := &eventRecorder{}
	registry := skills.NewRegistry(skillFixture("1.0.0", "PINNED BODY"))
	nodes := researchTestNodes(nil, events, WithToolLLM(llm), WithResearchTools(tool), WithSkillProvider(registry))

	in := researchTestInput()
	in.SkillPins = registry.Catalog()
	in.Skills = []skills.Skill{skillFixture("1.0.0", "STALE LEGACY BODY")}
	out, err := nodes.Research(context.Background(), in)
	if err != nil {
		t.Fatalf("research: %v", err)
	}
	if out.Brief.Status != "ok" {
		t.Fatalf("status = %s", out.Brief.Status)
	}
	first := llm.requestAt(t, 0)
	userPrompt := first.Messages[0].Content
	if !strings.Contains(userPrompt, "- research-playbook@1.0.0：研究清单") {
		t.Fatalf("catalog line missing: %s", userPrompt)
	}
	if strings.Contains(userPrompt, "PINNED BODY") || strings.Contains(userPrompt, "STALE LEGACY BODY") {
		t.Fatalf("prompt leaked a skill body: %s", userPrompt)
	}
	content := llm.requestAt(t, 1).Messages[2].Content
	if !strings.Contains(content, "PINNED BODY") || strings.Contains(content, "STALE LEGACY BODY") {
		t.Fatalf("pin did not win over legacy body: %s", content)
	}
	completed, ok := events.payloadFor("research.completed").(map[string]any)
	if !ok {
		t.Fatalf("completed payload = %#v", events.payloadFor("research.completed"))
	}
	loaded, ok := completed["skills_loaded"].([]loadedSkill)
	if !ok || len(loaded) != 1 || loaded[0].Name != "research-playbook" || loaded[0].Version != "1.0.0" || loaded[0].SHA256 == "" {
		t.Fatalf("skills_loaded = %#v", completed["skills_loaded"])
	}
}

func TestResearchMissingPinnedSkillDegrades(t *testing.T) {
	tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return guideToolOutput(t), nil
	}}
	llm := &fakeToolLLM{responses: []ToolCompletion{
		{ToolCalls: []ToolCall{toolCall("c1", researchSkillTool, `{"name":"research-playbook"}`)}},
		{Content: briefJSON(t, `{"status":"ok"}`)},
	}}
	events := &eventRecorder{}
	registry := skills.NewRegistry(skillFixture("2.0.0", "NEW BODY"))
	nodes := researchTestNodes(nil, events, WithToolLLM(llm), WithResearchTools(tool), WithSkillProvider(registry))

	in := researchTestInput()
	in.SkillPins = []skills.CatalogEntry{skillFixture("1.0.0", "OLD BODY").CatalogEntry()}
	out, err := nodes.Research(context.Background(), in)
	if err != nil {
		t.Fatalf("research: %v", err)
	}
	if out.Brief.Status != domain.ResearchDegraded {
		t.Fatalf("status = %s", out.Brief.Status)
	}
	toolContent := llm.requestAt(t, 1).Messages[2].Content
	if !strings.Contains(toolContent, "skill_unavailable") || !strings.Contains(toolContent, `"version":"1.0.0"`) {
		t.Fatalf("tool content = %s", toolContent)
	}
	degraded, ok := events.payloadFor("research.degraded").(map[string]any)
	if !ok {
		t.Fatalf("degraded payload = %#v", events.payloadFor("research.degraded"))
	}
	if degraded["reason"] != "skill_unavailable: research-playbook@1.0.0" {
		t.Fatalf("reason = %v", degraded["reason"])
	}
	completed := events.payloadFor("research.completed").(map[string]any)
	if loaded, ok := completed["skills_loaded"].([]loadedSkill); !ok || len(loaded) != 0 {
		t.Fatalf("skills_loaded = %#v", completed["skills_loaded"])
	}
}
