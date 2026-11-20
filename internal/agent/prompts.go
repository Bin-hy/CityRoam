package agent

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

//go:embed prompts/*.tmpl
var promptFiles embed.FS

var promptTemplates = template.Must(
	template.New("prompts").Funcs(template.FuncMap{"json": toJSON}).ParseFS(promptFiles, "prompts/*.tmpl"),
)

// ParsePromptData feeds parse.tmpl.
type ParsePromptData struct {
	Input string
	Today string
}

// DraftPromptData feeds draft.tmpl.
type DraftPromptData struct {
	Constraints json.RawMessage
	Candidates  json.RawMessage
	Days        int
}

// RepairPromptData feeds repair.tmpl.
type RepairPromptData struct {
	Constraints  json.RawMessage
	DayIndex     int
	CurrentDay   json.RawMessage
	Violations   json.RawMessage
	Alternatives json.RawMessage
}

// AdjustPromptData feeds adjust.tmpl.
type AdjustPromptData struct {
	Input        string
	Constraints  json.RawMessage
	BasePlan     json.RawMessage
	Scope        string
	AffectedDays []int
	Alternatives json.RawMessage
}

// AdjustIntentPromptData feeds intent.tmpl.
type AdjustIntentPromptData struct {
	Input       string
	Constraints json.RawMessage
	BasePlan    json.RawMessage
	Days        int
}

// ExplainPromptData feeds explain.tmpl.
type ExplainPromptData struct {
	Constraints json.RawMessage
	Plan        json.RawMessage
	Violations  json.RawMessage
	Diff        json.RawMessage
}

const researchPromptSeparator = "\n---USER---\n"

// RenderResearchPrompt splits research.tmpl into the system and user messages
// required by tool-calling Chat Completions.
func RenderResearchPrompt(data ResearchPromptData) (system, user string, err error) {
	rendered, err := RenderPrompt("research", data)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(rendered, researchPromptSeparator, 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("agent: research prompt is missing the system/user separator")
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

// RenderPrompt executes one of the embedded templates by short name
// ("parse", "draft", "repair", "adjust", "explain", "research").
func RenderPrompt(name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := promptTemplates.ExecuteTemplate(&buf, name+".tmpl", data); err != nil {
		return "", fmt.Errorf("agent: render prompt %q: %w", name, err)
	}
	return buf.String(), nil
}

func toJSON(value any) string {
	if raw, ok := value.(json.RawMessage); ok {
		return string(raw)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}
