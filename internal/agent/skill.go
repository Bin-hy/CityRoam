package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"trippilot/internal/skills"
)

const (
	researchSkillTool     = "load_skill"
	researchMaxSkillLoads = 3
)

// skillTool exposes skill bodies to the research loop. Pinned entries resolve
// through the provider on demand; legacy run state keeps its embedded bodies.
// A name with a pin always resolves through the pin, so a stale legacy body can
// never shadow an upgrade.
type skillTool struct {
	names    []string
	pins     map[string]skills.CatalogEntry
	legacy   map[string]skills.Skill
	provider skills.Provider
}

func newSkillTool(legacy []skills.Skill, pins []skills.CatalogEntry, provider skills.Provider) *skillTool {
	tool := &skillTool{
		pins:     make(map[string]skills.CatalogEntry, len(pins)),
		legacy:   make(map[string]skills.Skill, len(legacy)),
		provider: provider,
	}
	seen := make(map[string]struct{}, len(pins)+len(legacy))
	for _, pin := range pins {
		if pin.Name == "" {
			continue
		}
		tool.pins[pin.Name] = pin
		seen[pin.Name] = struct{}{}
	}
	for _, skill := range legacy {
		if skill.Name == "" {
			continue
		}
		tool.legacy[skill.Name] = skill
		seen[skill.Name] = struct{}{}
	}
	tool.names = make([]string, 0, len(seen))
	for name := range seen {
		tool.names = append(tool.names, name)
	}
	sort.Strings(tool.names)
	return tool
}

func skillToolEnabled(legacy []skills.Skill, pins []skills.CatalogEntry, provider skills.Provider) bool {
	if len(legacy) > 0 {
		return true
	}
	return len(pins) > 0 && provider != nil
}

func (t *skillTool) Name() string { return researchSkillTool }

func (t *skillTool) Description() string {
	return "加载某个技能的完整说明。系统提示只提供技能名称与用途；当某条技能与当前研究任务相关时，调用本工具读取正文，再按其中的步骤处理。"
}

func (t *skillTool) InputSchema() json.RawMessage {
	properties := map[string]any{
		"name": map[string]any{
			"type":        "string",
			"description": "技能名称，只能从 enum 中选择",
		},
	}
	if len(t.names) > 0 {
		properties["name"].(map[string]any)["enum"] = t.names
	}
	schema, err := json.Marshal(map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             []string{"name"},
		"additionalProperties": false,
	})
	if err != nil {
		return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	}
	return schema
}

func (t *skillTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var request struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &request); err != nil || strings.TrimSpace(request.Name) == "" {
		return skillToolError("invalid_input", "需要提供技能名称"), nil
	}
	name := strings.TrimSpace(request.Name)
	if pin, ok := t.pins[name]; ok {
		if t.provider == nil {
			return skillUnavailableError(pin.Name, pin.Version), nil
		}
		skill, err := t.provider.Resolve(ctx, pin)
		if err != nil {
			return skillUnavailableError(pin.Name, pin.Version), nil
		}
		return skillPayloadJSON(skill)
	}
	if skill, ok := t.legacy[name]; ok {
		return skillPayloadJSON(skill)
	}
	return skillToolError("unknown_skill", "技能不存在或未启用"), nil
}

func skillPayloadJSON(skill skills.Skill) (json.RawMessage, error) {
	payload, err := json.Marshal(skillPayload{
		Name:         skill.Name,
		Version:      skill.Version,
		SHA256:       skill.SHA256,
		Instructions: skill.Body,
	})
	if err != nil {
		return skillToolError("bad_response", "技能内容无法编码"), nil
	}
	return payload, nil
}

type skillPayload struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	SHA256       string `json:"sha256"`
	Instructions string `json:"instructions"`
}

func skillToolError(code, message string) json.RawMessage {
	payload, err := json.Marshal(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
	if err != nil {
		return json.RawMessage(`{"error":{"code":"tool_error","message":"技能工具失败"}}`)
	}
	return payload
}

// skillUnavailableError reports the exact pin that failed so the run can name
// it in the degradation reason without exposing paths or bodies.
func skillUnavailableError(name, version string) json.RawMessage {
	payload, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    "skill_unavailable",
			"message": "技能版本不可用",
			"skill":   map[string]string{"name": name, "version": version},
		},
	})
	if err != nil {
		return json.RawMessage(`{"error":{"code":"skill_unavailable","message":"技能版本不可用"}}`)
	}
	return payload
}

func renderSkillContent(output json.RawMessage) (string, skillPayload, bool) {
	var payload skillPayload
	if err := json.Unmarshal(output, &payload); err != nil {
		return "", skillPayload{}, false
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Instructions) == "" {
		return "", skillPayload{}, false
	}
	content := fmt.Sprintf("<skill name=%q version=%q sha256=%q>\n%s\n</skill>",
		payload.Name, payload.Version, payload.SHA256, payload.Instructions)
	return content, payload, true
}

func skillCatalog(pinned []skills.Skill) []skills.CatalogEntry {
	if len(pinned) == 0 {
		return nil
	}
	out := make([]skills.CatalogEntry, 0, len(pinned))
	for _, skill := range pinned {
		out = append(out, skill.CatalogEntry())
	}
	return out
}
