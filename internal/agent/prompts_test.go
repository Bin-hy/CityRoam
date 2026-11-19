package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestPromptSchemas(t *testing.T) {
	t.Run("templates render", func(t *testing.T) {
		cases := []struct {
			name string
			data any
			want string
		}{
			{"parse", ParsePromptData{Input: "北京两天，预算1000", Today: "2026-11-19"}, "北京两天"},
			{"draft", DraftPromptData{
				Constraints: json.RawMessage(`{"city":"beijing","days":2}`),
				Candidates:  json.RawMessage(`[{"id":"forbidden-city","name":"故宫"}]`),
				Days:        2,
			}, "forbidden-city"},
			{"repair", RepairPromptData{
				Constraints:  json.RawMessage(`{"city":"beijing","days":2}`),
				DayIndex:     1,
				CurrentDay:   json.RawMessage(`{"items":[]}`),
				Violations:   json.RawMessage(`[{"code":"C1"}]`),
				Alternatives: json.RawMessage(`[]`),
			}, "第 1 天"},
			{"adjust", AdjustPromptData{
				Input:        "第二天轻松一点",
				Constraints:  json.RawMessage(`{}`),
				BasePlan:     json.RawMessage(`{"days":[]}`),
				Scope:        "day",
				AffectedDays: []int{1},
			}, "第二天轻松一点"},
			{"explain", ExplainPromptData{
				Constraints: json.RawMessage(`{}`),
				Plan:        json.RawMessage(`{"days":[]}`),
				Violations:  json.RawMessage(`[]`),
				Diff:        json.RawMessage(`{}`),
			}, "explanations"},
		}

		for _, tc := range cases {
			rendered, err := RenderPrompt(tc.name, tc.data)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if !strings.Contains(rendered, tc.want) {
				t.Fatalf("%s prompt missing %q:\n%s", tc.name, tc.want, rendered)
			}
			if strings.Contains(rendered, "<no value>") {
				t.Fatalf("%s prompt has unset fields:\n%s", tc.name, rendered)
			}
		}

		if _, err := RenderPrompt("missing", nil); err == nil {
			t.Fatal("want error for unknown template")
		}
	})

	t.Run("schemas compile and validate", func(t *testing.T) {
		constraint := compileSchema(t, ConstraintSchema)
		assertValid(t, constraint, `{"city":"beijing","days":2,"pace":"relaxed","meals":{"enabled":true},"missing":[]}`)
		assertInvalid(t, constraint, `{"days":9}`)
		assertInvalid(t, constraint, `{"pace":"extreme"}`)
		assertInvalid(t, constraint, `{"unexpected":true}`)

		draft := compileSchema(t, DraftPlanSchema)
		assertValid(t, draft, `{"days":[{"date":"2026-11-20","items":[{"poi_id":"forbidden-city","type":"poi","duration_min":120}]}],"explanations":[{"poi_id":"forbidden-city","text":"经典"}]}`)
		assertInvalid(t, draft, `{"days":[{"items":[{"poi_id":"x","type":"hotel","duration_min":60}]}]}`)
		assertInvalid(t, draft, `{"days":[]}`)

		patch := compileSchema(t, DayPatchSchema)
		assertValid(t, patch, `{"day_index":1,"items":[{"poi_id":"temple-of-heaven","type":"poi","duration_min":90}],"reason":"减少步行"}`)
		assertInvalid(t, patch, `{"day_index":1,"items":[]}`)
		assertInvalid(t, patch, `{"day_index":1,"items":[{"poi_id":"x","type":"poi","duration_min":90}]}`)
	})
}

func compileSchema(t *testing.T, raw json.RawMessage) *jsonschema.Resolved {
	t.Helper()
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}
	return resolved
}

func assertValid(t *testing.T, schema *jsonschema.Resolved, instance string) {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(instance), &value); err != nil {
		t.Fatalf("instance: %v", err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("want valid %s: %v", instance, err)
	}
}

func assertInvalid(t *testing.T, schema *jsonschema.Resolved, instance string) {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(instance), &value); err != nil {
		t.Fatalf("instance: %v", err)
	}
	if err := schema.Validate(value); err == nil {
		t.Fatalf("want invalid %s", instance)
	}
}
