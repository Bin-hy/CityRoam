package skillseval

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
	"trippilot/internal/skills"
)

func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	entries := []struct {
		name        string
		description string
	}{
		{"museum-closure", "用于博物馆闭馆日或周一排期时核对开放时间。"},
		{"rainy-day", "用于预报有雨、需要室内备选时调整行程。"},
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.name, "1.0.0", skills.FileName)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		content := "---\nname: " + entry.name + "\ndescription: " + entry.description +
			"\nversion: 1.0.0\n---\n\n- 先核对开放时间。\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write skill: %v", err)
		}
	}
	lock, _, err := skills.BuildLock(dir)
	if err != nil {
		t.Fatalf("build lock: %v", err)
	}
	if err := skills.WriteLock(dir, lock); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	return dir
}

type fakeToolLLM struct {
	responses []agent.ToolCompletion
	err       error
	calls     int
}

func (f *fakeToolLLM) CompleteWithTools(_ context.Context, _ agent.ToolCompletionRequest) (agent.ToolCompletion, agent.Usage, error) {
	index := f.calls
	f.calls++
	if f.err != nil {
		return agent.ToolCompletion{}, agent.Usage{}, f.err
	}
	if index >= len(f.responses) {
		return agent.ToolCompletion{Content: `{"status":"ok"}`}, agent.Usage{}, nil
	}
	return f.responses[index], agent.Usage{}, nil
}

func loadCall(name string) agent.ToolCompletion {
	return agent.ToolCompletion{ToolCalls: []agent.ToolCall{{
		ID:        "call-" + name,
		Name:      "load_skill",
		Arguments: json.RawMessage(`{"name":"` + name + `"}`),
	}}}
}

func evalCases() []Case {
	candidates := []domain.POI{{ID: "poi-0", Name: "国家博物馆", City: "beijing"}}
	return []Case{
		{ID: "museum", Input: "周一去国家博物馆", City: "beijing", Days: 1, Candidates: candidates, ExpectLoaded: []string{"museum-closure"}},
		{ID: "rain", Input: "下雨怎么办", City: "beijing", Days: 2, Candidates: candidates, ExpectLoaded: []string{}},
		{ID: "model-error", Input: "普通一天", City: "beijing", Days: 1, Candidates: candidates, ExpectLoaded: []string{"museum-closure"}},
	}
}

func TestRunScoresExactLoadedSets(t *testing.T) {
	llm := &fakeToolLLM{responses: []agent.ToolCompletion{
		loadCall("museum-closure"),
		{Content: `{"status":"ok"}`},
		loadCall("rainy-day"),
		{Content: `{"status":"ok"}`},
	}}
	report, err := Run(context.Background(), Config{
		SkillsDir: writeFixture(t),
		Cases:     evalCases(),
		LLM:       llm,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.Passed != 1 || report.Failed != 2 {
		t.Fatalf("report = %+v", report)
	}
	if len(report.Results) != 3 {
		t.Fatalf("results = %d", len(report.Results))
	}
	if got := report.Results[0]; got.Err != nil || strings.Join(got.Loaded, ",") != "museum-closure" {
		t.Fatalf("passing case = %+v", got)
	}
	if got := report.Results[1]; got.Err == nil || !strings.Contains(got.Err.Error(), "unexpected rainy-day") {
		t.Fatalf("distinguishing case = %+v", got)
	}
	if got := report.Results[2]; got.Err == nil || !strings.Contains(got.Err.Error(), "missing museum-closure") {
		t.Fatalf("no-load case = %+v", got)
	}
}

func TestRunPropagatesModelErrors(t *testing.T) {
	report, err := Run(context.Background(), Config{
		SkillsDir: writeFixture(t),
		Cases:     evalCases()[:1],
		LLM:       &fakeToolLLM{err: errors.New("upstream unavailable")},
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.Failed != 1 || report.Results[0].Err == nil {
		t.Fatalf("report = %+v", report)
	}
	if !strings.Contains(report.Results[0].Err.Error(), "missing museum-closure") {
		t.Fatalf("err = %v", report.Results[0].Err)
	}
}

func TestLoadCases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cases.json")
	if err := os.WriteFile(path, []byte(`[{"id":"a","input":"北京一天","city":"beijing","days":1,"expect_loaded":[]}]`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cases, err := LoadCases(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cases) != 1 || cases[0].ID != "a" || len(cases[0].ExpectLoaded) != 0 {
		t.Fatalf("cases = %+v", cases)
	}

	if _, err := LoadCases(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected missing file error")
	}
	if err := os.WriteFile(path, []byte(`[]`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadCases(path); err == nil {
		t.Fatal("expected empty cases error")
	}
	if err := os.WriteFile(path, []byte(`[{"id":"a","input":"x"},{"id":"a","input":"y"}]`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadCases(path); err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestSetDifference(t *testing.T) {
	if got := setDifference([]string{"a", "b"}, []string{"b", "a"}); got != "" {
		t.Fatalf("equal sets = %q", got)
	}
	if got := setDifference([]string{"a"}, []string{}); got != "missing a" {
		t.Fatalf("missing = %q", got)
	}
	if got := setDifference(nil, []string{"b"}); got != "unexpected b" {
		t.Fatalf("extra = %q", got)
	}
}
