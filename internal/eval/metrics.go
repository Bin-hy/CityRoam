package eval

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
)

// RunMetrics is stored in eval_runs.metrics and rendered by the dashboard.
type RunMetrics struct {
	HardPass       bool    `json:"hard_pass"`
	RepairRounds   int     `json:"repair_rounds"`
	LatencyMS      int64   `json:"latency_ms"`
	POIValid       bool    `json:"poi_valid"`
	JSONValidRate  float64 `json:"json_valid_rate"`
	JSONCalls      int     `json:"json_calls,omitempty"`
	JSONAttempts   int     `json:"json_attempts,omitempty"`
	Blocking       int     `json:"blocking"`
	Warnings       int     `json:"warnings"`
	Tokens         int     `json:"total_tokens,omitempty"`
	ResearchStatus string  `json:"research_status"`
	ResearchMS     int64   `json:"research_ms"`
	ToolCalls      int     `json:"tool_calls"`
	LodgingYuan    int     `json:"lodging_yuan"`
	GuideMatches   int     `json:"guide_matches"`
}

// Summary aggregates one batch.
type Summary struct {
	Cases           int     `json:"cases"`
	Passed          int     `json:"passed"`
	HardPassRate    float64 `json:"hard_pass_rate"`
	AvgRepairRounds float64 `json:"avg_repair_rounds"`
	P95LatencyMS    int64   `json:"p95_latency_ms"`
	POIValidRate    float64 `json:"poi_valid_rate"`
	JSONValidRate   float64 `json:"json_valid_rate"`
}

// RunResult is the per-case outcome of a batch.
type RunResult struct {
	CaseID    string     `json:"case_id"`
	RunID     string     `json:"run_id,omitempty"`
	Days      int        `json:"days,omitempty"`
	Passed    bool       `json:"passed"`
	Reason    string     `json:"reason,omitempty"`
	Metrics   RunMetrics `json:"metrics"`
	Completed bool       `json:"completed"`
}

// Report is the output of one full batch.
type Report struct {
	GeneratedAt time.Time   `json:"generated_at"`
	Notes       []string    `json:"notes,omitempty"`
	Results     []RunResult `json:"results"`
	Summary     Summary     `json:"summary"`
}

var avoidAliases = map[string][]string{
	"爬山":   {"hiking"},
	"主题公园": {"theme_park"},
	"人多":   {"crowded"},
}

// Evaluate compares one finished run against its expected behavior.
func Evaluate(c Case, state *agent.TripState) (RunMetrics, []string) {
	reasons := make([]string, 0)
	metrics := RunMetrics{}

	if state == nil {
		reasons = append(reasons, "运行没有可读取的状态")
		return metrics, reasons
	}

	for _, violation := range state.Violations {
		switch violation.Level {
		case domain.LevelBlocking:
			metrics.Blocking++
		case domain.LevelWarning:
			metrics.Warnings++
		}
	}

	metrics.RepairRounds = state.Round
	metrics.Tokens = state.LLM.TotalTokens
	metrics.JSONCalls = state.LLM.Calls
	metrics.JSONAttempts = state.LLM.Attempts
	applyResearchMetrics(&metrics, state)
	if state.LLM.Attempts > 0 {
		metrics.JSONValidRate = float64(state.LLM.Calls) / float64(state.LLM.Attempts)
		if metrics.JSONValidRate > 1 {
			metrics.JSONValidRate = 1
		}
	} else if len(state.Plan.Days) > 0 {
		metrics.JSONValidRate = 1
	}

	days := len(state.Plan.Days)
	if days != c.Expected.Days {
		reasons = append(reasons, fmt.Sprintf("天数应为 %d，实际 %d", c.Expected.Days, days))
	}

	if len(state.Plan.Days) == 0 {
		reasons = append(reasons, "没有生成行程")
	}

	metrics.POIValid = poiValid(state)
	if !metrics.POIValid {
		reasons = append(reasons, "行程包含候选集之外的 POI")
	}

	codeSet := make(map[string]struct{}, len(c.Expected.HardRules))
	for _, code := range c.Expected.HardRules {
		codeSet[code] = struct{}{}
	}
	for _, violation := range state.Violations {
		if _, required := codeSet[violation.Code]; required {
			reasons = append(reasons, fmt.Sprintf("违反硬规则 %s：%s", violation.Code, violation.Message))
		}
	}
	if metrics.Blocking > 0 {
		reasons = append(reasons, fmt.Sprintf("仍有 %d 条阻塞违规", metrics.Blocking))
	}

	for _, name := range c.Expected.MustVisit {
		if !planContains(state.Plan, name) {
			reasons = append(reasons, "缺少必去点："+name)
		}
	}

	if hit := avoidHit(state, c.Expected.Avoid); hit != "" {
		reasons = append(reasons, "命中需避开的内容："+hit)
	}

	if c.Expected.ResearchStatus != "" && metrics.ResearchStatus != c.Expected.ResearchStatus {
		reasons = append(reasons, fmt.Sprintf("研究状态应为 %s，实际 %s", c.Expected.ResearchStatus, metrics.ResearchStatus))
	}
	if c.Expected.Lodging && metrics.LodgingYuan <= 0 {
		reasons = append(reasons, "住宿费用未计入预算")
	}

	metrics.HardPass = len(reasons) == 0
	return metrics, reasons
}

func applyResearchMetrics(metrics *RunMetrics, state *agent.TripState) {
	metrics.ResearchStatus = "skipped"
	metrics.LodgingYuan = state.Plan.Totals.LodgingYuan
	if state.Research == nil {
		return
	}
	if state.Research.Status != "" {
		metrics.ResearchStatus = string(state.Research.Status)
	}
	metrics.ResearchMS = state.Research.DurationMS
	metrics.ToolCalls = state.Research.ToolCalls
	metrics.GuideMatches = len(state.Research.Guides)
}

func poiValid(state *agent.TripState) bool {
	if len(state.Plan.Days) == 0 {
		return false
	}
	known := make(map[string]struct{}, len(state.Candidates))
	for _, poi := range state.Candidates {
		known[poi.ID] = struct{}{}
	}
	for _, day := range state.Plan.Days {
		for _, block := range day.Blocks {
			if block.Type != domain.BlockTypePOI && block.Type != domain.BlockTypeMeal {
				continue
			}
			if block.POIID == "" {
				return false
			}
			if _, found := known[block.POIID]; !found {
				return false
			}
		}
	}
	return true
}

func planContains(plan domain.Itinerary, name string) bool {
	want := normalize(name)
	if want == "" {
		return true
	}
	for _, day := range plan.Days {
		for _, block := range day.Blocks {
			values := []string{block.Title, block.POIID}
			for _, value := range values {
				got := normalize(value)
				if got == "" {
					continue
				}
				if strings.Contains(got, want) || strings.Contains(want, got) {
					return true
				}
			}
		}
	}
	return false
}

func avoidHit(state *agent.TripState, avoid []string) string {
	if len(avoid) == 0 {
		return ""
	}
	known := make(map[string]domain.POI, len(state.Candidates))
	for _, poi := range state.Candidates {
		known[poi.ID] = poi
	}
	for _, term := range avoid {
		needle := normalize(term)
		if needle == "" {
			continue
		}
		aliases := append([]string{needle}, avoidAliases[term]...)
		for _, day := range state.Plan.Days {
			for _, block := range day.Blocks {
				poi := known[block.POIID]
				haystack := []string{block.Title, poi.Name, poi.Category}
				haystack = append(haystack, poi.Tags...)
				for _, value := range haystack {
					got := normalize(value)
					for _, alias := range aliases {
						if alias != "" && strings.Contains(got, alias) {
							return term + "（" + block.Title + "）"
						}
					}
				}
			}
		}
	}
	return ""
}

func normalize(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), ""))
}

// Summarize aggregates per-case results into batch metrics.
func Summarize(results []RunResult) Summary {
	summary := Summary{Cases: len(results)}
	if len(results) == 0 {
		return summary
	}

	latencies := make([]int64, 0, len(results))
	threeDay := make([]int64, 0, len(results))
	var rounds int
	var poiValid, jsonCalls, jsonAttempts int
	for _, result := range results {
		if result.Passed {
			summary.Passed++
		}
		latencies = append(latencies, result.Metrics.LatencyMS)
		if result.Days == 3 {
			threeDay = append(threeDay, result.Metrics.LatencyMS)
		}
		rounds += result.Metrics.RepairRounds
		if result.Metrics.POIValid {
			poiValid++
		}
		jsonCalls += result.Metrics.JSONCalls
		jsonAttempts += result.Metrics.JSONAttempts
	}

	summary.HardPassRate = float64(summary.Passed) / float64(len(results))
	summary.AvgRepairRounds = float64(rounds) / float64(len(results))
	summary.POIValidRate = float64(poiValid) / float64(len(results))
	if jsonAttempts > 0 {
		summary.JSONValidRate = float64(jsonCalls) / float64(jsonAttempts)
	}
	if len(threeDay) > 0 {
		latencies = threeDay
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	index := (95*len(latencies)+99)/100 - 1
	if index < 0 {
		index = 0
	}
	summary.P95LatencyMS = latencies[index]
	return summary
}

// Markdown renders a reproducible report for docs/eval-report.md.
func Markdown(report Report) string {
	var b strings.Builder
	b.WriteString("# TripPilot 评测报告\n\n")
	fmt.Fprintf(&b, "- 生成时间：%s\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- 用例数：%d\n\n", report.Summary.Cases)

	if len(report.Notes) > 0 {
		b.WriteString("## 运行环境\n\n")
		for _, note := range report.Notes {
			fmt.Fprintf(&b, "- %s\n", note)
		}
		b.WriteString("\n")
	}

	b.WriteString("| 指标 | 值 |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| 硬约束满足率 | %.1f%% |\n", report.Summary.HardPassRate*100)
	fmt.Fprintf(&b, "| 通过用例 | %d/%d |\n", report.Summary.Passed, report.Summary.Cases)
	fmt.Fprintf(&b, "| 平均修复轮数 | %.2f |\n", report.Summary.AvgRepairRounds)
	fmt.Fprintf(&b, "| 3 天 p95 延迟 | %d ms |\n", report.Summary.P95LatencyMS)
	fmt.Fprintf(&b, "| POI 有效率 | %.1f%% |\n", report.Summary.POIValidRate*100)
	fmt.Fprintf(&b, "| JSON 合法率 | %.1f%% |\n", report.Summary.JSONValidRate*100)
	b.WriteString("| 人工评分 | 待填 |\n\n")

	b.WriteString("| 用例 | 结果 | 硬约束 | 修复轮数 | 延迟 | POI | JSON | 研究 | 研究耗时 | 工具调用 | 住宿 | 攻略匹配 | 说明 |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, result := range report.Results {
		status := "失败"
		if result.Passed {
			status = "通过"
		}
		hard := "否"
		if result.Metrics.HardPass {
			hard = "是"
		}
		poi := "否"
		if result.Metrics.POIValid {
			poi = "是"
		}
		reason := result.Reason
		if reason == "" {
			reason = "-"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d ms | %s | %.1f%% | %s | %d ms | %d | %d | %d | %s |\n",
			result.CaseID,
			status,
			hard,
			result.Metrics.RepairRounds,
			result.Metrics.LatencyMS,
			poi,
			result.Metrics.JSONValidRate*100,
			result.Metrics.ResearchStatus,
			result.Metrics.ResearchMS,
			result.Metrics.ToolCalls,
			result.Metrics.LodgingYuan,
			result.Metrics.GuideMatches,
			strings.ReplaceAll(reason, "|", "/"),
		)
	}
	return b.String()
}

// DryRunMarkdown lists the loaded cases without executing them.
func DryRunMarkdown(cases []Case) string {
	var b strings.Builder
	b.WriteString("# TripPilot 评测预检\n\n")
	fmt.Fprintf(&b, "- 用例数：%d\n\n", len(cases))
	b.WriteString("| 用例 | 天数 | 必去 | 避开 | 标签 | 研究 | 研究耗时 | 工具调用 | 住宿 | 攻略匹配 |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, item := range cases {
		must := strings.Join(item.Expected.MustVisit, "、")
		if must == "" {
			must = "-"
		}
		avoid := strings.Join(item.Expected.Avoid, "、")
		if avoid == "" {
			avoid = "-"
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | skipped | 0 ms | 0 | 0 | 0 |\n",
			item.CaseID, item.Expected.Days, must, avoid, strings.Join(item.Tags, ","))
	}
	return b.String()
}
