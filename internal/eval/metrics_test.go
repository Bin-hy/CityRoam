package eval

import (
	"strings"
	"testing"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
)

func sampleState() *agent.TripState {
	return &agent.TripState{
		Plan: domain.Itinerary{
			City: "beijing",
			Days: []domain.DayPlan{{
				Date: "2026-11-21",
				Blocks: []domain.Block{
					{ID: "b1", Type: domain.BlockTypePOI, POIID: "forbidden-city", Title: "故宫博物院"},
					{ID: "b2", Type: domain.BlockTypePOI, POIID: "temple-of-heaven", Title: "天坛公园"},
				},
			}},
		},
		Candidates: []domain.POI{
			{ID: "forbidden-city", Name: "故宫博物院"},
			{ID: "temple-of-heaven", Name: "天坛公园"},
		},
		LLM: agent.Usage{Calls: 3, Attempts: 4},
	}
}

func TestEvaluatePassesExpectedCase(t *testing.T) {
	item := Case{
		CaseID: "case-1",
		Expected: Expected{
			Days:      1,
			HardRules: []string{"C5"},
			MustVisit: []string{"故宫博物院"},
		},
	}
	metrics, reasons := Evaluate(item, sampleState())
	if !metrics.HardPass {
		t.Fatalf("expected pass, reasons: %v", reasons)
	}
	if !metrics.POIValid {
		t.Fatal("expected POI valid")
	}
	if metrics.JSONCalls != 3 || metrics.JSONAttempts != 4 {
		t.Fatalf("JSON usage = %d/%d", metrics.JSONCalls, metrics.JSONAttempts)
	}
	if metrics.JSONValidRate != 0.75 {
		t.Fatalf("JSON rate = %v, want 0.75", metrics.JSONValidRate)
	}
}

func TestEvaluateFailsOnBlockingAndMissingMustVisit(t *testing.T) {
	state := sampleState()
	state.Plan.Days[0].Blocks = state.Plan.Days[0].Blocks[:1]
	state.Violations = []domain.Violation{{
		Code:    "C5",
		Level:   domain.LevelBlocking,
		Message: "预算超出",
	}}
	item := Case{CaseID: "case-2", Expected: Expected{Days: 1, HardRules: []string{"C5"}, MustVisit: []string{"天坛公园"}}}

	metrics, reasons := Evaluate(item, state)
	if metrics.HardPass {
		t.Fatal("expected failure")
	}
	if metrics.Blocking != 1 {
		t.Fatalf("blocking = %d", metrics.Blocking)
	}
	joined := strings.Join(reasons, "；")
	if !strings.Contains(joined, "预算超出") || !strings.Contains(joined, "缺少必去点") {
		t.Fatalf("reasons = %v", reasons)
	}
}

func TestEvaluateRejectsUnknownPOIAndAvoidedTag(t *testing.T) {
	state := sampleState()
	state.Candidates = state.Candidates[:1]
	state.Plan.Days[0].Blocks[0].POIID = "unknown"
	state.Plan.Days[0].Blocks[1].Title = "香山公园"
	state.Plan.Days[0].Blocks[1].POIID = "temple-of-heaven"
	state.Candidates = append(state.Candidates, domain.POI{ID: "temple-of-heaven", Name: "香山公园", Tags: []string{"hiking"}})

	item := Case{CaseID: "case-3", Expected: Expected{Days: 1, Avoid: []string{"爬山"}}}
	metrics, reasons := Evaluate(item, state)
	if metrics.POIValid {
		t.Fatal("expected invalid POI set")
	}
	if metrics.HardPass {
		t.Fatal("expected failure")
	}
	if !strings.Contains(strings.Join(reasons, "；"), "命中需避开") {
		t.Fatalf("reasons = %v", reasons)
	}
}

func TestEvaluateResearchMetricsDefaultToSkipped(t *testing.T) {
	metrics, _ := Evaluate(Case{CaseID: "case-skip", Expected: Expected{Days: 1}}, sampleState())
	if metrics.ResearchStatus != "skipped" || metrics.ResearchMS != 0 || metrics.ToolCalls != 0 || metrics.LodgingYuan != 0 || metrics.GuideMatches != 0 {
		t.Fatalf("research metrics = %+v", metrics)
	}
}

func TestEvaluateReadsResearchAndLodging(t *testing.T) {
	state := sampleState()
	state.Plan.Totals.LodgingYuan = 880
	state.Research = &domain.ResearchBrief{
		Status:     domain.ResearchOK,
		DurationMS: 1250,
		ToolCalls:  3,
		Guides:     []domain.GuideSignal{{POIID: "forbidden-city", Weight: 0.7}},
	}
	metrics, _ := Evaluate(Case{CaseID: "case-research", Expected: Expected{Days: 1}}, state)
	if metrics.ResearchStatus != "ok" || metrics.ResearchMS != 1250 || metrics.ToolCalls != 3 || metrics.LodgingYuan != 880 || metrics.GuideMatches != 1 {
		t.Fatalf("research metrics = %+v", metrics)
	}
}

func TestEvaluateChecksResearchExpectations(t *testing.T) {
	state := sampleState()
	state.Research = &domain.ResearchBrief{Status: domain.ResearchDegraded}
	item := Case{CaseID: "case-research-expect", Expected: Expected{Days: 1, ResearchStatus: "ok", Lodging: true}}
	metrics, reasons := Evaluate(item, state)
	if metrics.HardPass {
		t.Fatal("expected failure")
	}
	joined := strings.Join(reasons, "；")
	if !strings.Contains(joined, "研究状态应为 ok") || !strings.Contains(joined, "住宿费用未计入预算") {
		t.Fatalf("reasons = %v", reasons)
	}
}

func TestMarkdownIncludesResearchColumns(t *testing.T) {
	report := Report{
		Results: []RunResult{{CaseID: "case-1", Metrics: RunMetrics{ResearchStatus: "skipped"}}},
	}
	body := Markdown(report)
	for _, header := range []string{"研究", "研究耗时", "工具调用", "住宿", "攻略匹配"} {
		if !strings.Contains(body, header) {
			t.Fatalf("missing header %q in %s", header, body)
		}
	}
	if !strings.Contains(body, "| case-1 |") || !strings.Contains(body, "| skipped |") {
		t.Fatalf("missing skipped row in %s", body)
	}
}

func TestDryRunMarkdownIncludesResearchColumns(t *testing.T) {
	body := DryRunMarkdown([]Case{{CaseID: "case-1", Expected: Expected{Days: 2}}})
	for _, header := range []string{"研究", "研究耗时", "工具调用", "住宿", "攻略匹配"} {
		if !strings.Contains(body, header) {
			t.Fatalf("missing header %q in %s", header, body)
		}
	}
	if !strings.Contains(body, "| skipped | 0 ms | 0 | 0 | 0 |") {
		t.Fatalf("missing default research cells in %s", body)
	}
}

func TestSummarizeComputesP95(t *testing.T) {
	results := make([]RunResult, 20)
	for i := range results {
		results[i] = RunResult{
			Passed: i%2 == 0,
			Metrics: RunMetrics{
				LatencyMS:    int64((i + 1) * 1000),
				POIValid:     true,
				JSONCalls:    1,
				JSONAttempts: 1,
				RepairRounds: i % 3,
			},
		}
	}
	summary := Summarize(results)
	if summary.Cases != 20 || summary.Passed != 10 {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.HardPassRate != 0.5 {
		t.Fatalf("hard pass rate = %v", summary.HardPassRate)
	}
	if summary.P95LatencyMS != 19000 {
		t.Fatalf("p95 = %d, want 19000", summary.P95LatencyMS)
	}
	if summary.POIValidRate != 1 || summary.JSONValidRate != 1 {
		t.Fatalf("rates = %+v", summary)
	}
}

func TestSummarizeP95UsesThreeDayCases(t *testing.T) {
	results := []RunResult{
		{Days: 3, Metrics: RunMetrics{LatencyMS: 1000}},
		{Days: 3, Metrics: RunMetrics{LatencyMS: 3000}},
		{Days: 1, Metrics: RunMetrics{LatencyMS: 9000}},
	}
	summary := Summarize(results)
	if summary.P95LatencyMS != 3000 {
		t.Fatalf("p95 = %d, want 3000 from 3-day cases", summary.P95LatencyMS)
	}
}
