package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"trippilot/internal/domain"
	"trippilot/internal/domain/scheduler"
	"trippilot/internal/domain/validator"
	"trippilot/internal/skills"
)

func graphPlan() domain.Itinerary {
	return domain.Itinerary{
		City: "beijing",
		Days: []domain.DayPlan{{
			Date:      "2026-11-20",
			StartTime: "09:00",
			EndTime:   "11:30",
			Blocks: []domain.Block{
				{ID: "b1", Type: domain.BlockTypePOI, POIID: "poi-0", Title: "poi-0景点0", Start: "09:00", End: "10:30", DurationMin: 90, CostYuan: 30},
			},
			DayCostYuan: 30,
		}},
		Totals: domain.Totals{CostYuan: 30, POICount: 1, WalkingKM: 1.5},
	}
}

func graphDraftReply() json.RawMessage {
	return json.RawMessage(`{"days":[{"date":"2026-11-20","items":[{"poi_id":"poi-0","type":"poi","duration_min":90}]}],"explanations":[{"poi_id":"poi-0","text":"经典打卡点"}]}`)
}

func graphRepairReply(poiID string) json.RawMessage {
	payload, _ := json.Marshal(map[string]any{
		"day_index": 0,
		"items":     []map[string]any{{"poi_id": poiID, "type": "poi", "duration_min": 90}},
		"reason":    "换成同类地点",
	})
	return payload
}

func graphNodes(llm LLMClient, events *eventRecorder, validator Validator) *Nodes {
	source := &fakeSource{fn: func(context.Context, domain.POIQuery) ([]domain.POI, error) {
		return makePOIs("poi", 2), nil
	}}
	return NewNodes(llm, events,
		WithSnapshotSource(source),
		WithScheduler(&fakeScheduler{plan: graphPlan()}),
		WithValidator(validator),
		WithNodeClock(func() time.Time { return time.Date(2026, 11, 19, 10, 0, 0, 0, time.Local) }),
		WithNodeLogger(discardLogger()),
	)
}

func TestGraph(t *testing.T) {
	t.Run("runs parse to present", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1}`),
			graphDraftReply(),
		}}
		events := &eventRecorder{}
		nodes := graphNodes(llm, events, &fakeValidator{})
		graph, err := NewGraph(context.Background(), nodes)
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}

		out, err := graph.Run(context.Background(), &TripState{RunID: "run-1", Input: "北京一天"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.Status != StatusAwaitingUser {
			t.Fatalf("status = %s", out.Status)
		}
		if out.Constraints.City != "beijing" || out.Constraints.Days != 1 {
			t.Fatalf("constraints = %+v", out.Constraints)
		}
		if len(out.Candidates) != 2 || out.Plan.Totals.CostYuan != 30 {
			t.Fatalf("state = %+v", out)
		}
		if out.Snapshot == nil || out.Snapshot.Seq != 1 || out.Snapshot.Reason != "initial" {
			t.Fatalf("snapshot = %+v", out.Snapshot)
		}
		if len(out.Snapshot.Explanations) != 1 || out.Snapshot.Explanations[0].Text != "经典打卡点" {
			t.Fatalf("explanations = %+v", out.Snapshot.Explanations)
		}
		if len(out.History) != 1 || out.History[0].Blocking != 0 {
			t.Fatalf("history = %+v", out.History)
		}

		types := events.types()
		for _, want := range []string{"constraint.parsed", "candidate.retrieved", "plan.drafted", "validation.result", "plan.updated"} {
			if !containsString(types, want) {
				t.Fatalf("missing event %s in %v", want, types)
			}
		}
		if containsString(types, "repair.started") {
			t.Fatalf("unexpected repair: %v", types)
		}
		if len(llm.calls) != 2 {
			t.Fatalf("llm calls = %d", len(llm.calls))
		}
	})

	t.Run("stops after parse when critical fields are missing", func(t *testing.T) {
		llm := &fakeLLM{reply: json.RawMessage(`{"city":"","days":0}`)}
		events := &eventRecorder{}
		nodes := graphNodes(llm, events, &fakeValidator{})
		graph, err := NewGraph(context.Background(), nodes)
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}

		out, err := graph.Run(context.Background(), &TripState{RunID: "run-2", Input: "去玩"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.Status != StatusAwaitingUser || len(out.Missing) != 2 {
			t.Fatalf("state = %+v", out)
		}
		if out.Snapshot != nil || len(out.Candidates) != 0 {
			t.Fatalf("graph continued past parse: %+v", out)
		}
		if got := events.types(); len(got) != 1 || got[0] != "constraint.needs_input" {
			t.Fatalf("events = %v", got)
		}
	})

	t.Run("repairs then stops when violations repeat", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1}`),
			graphDraftReply(),
			graphRepairReply("poi-1"),
			graphRepairReply("poi-1"),
		}}
		events := &eventRecorder{}
		blocking := []domain.Violation{{Code: "C4", Level: domain.LevelBlocking, Day: 1, Message: "超出每日时长"}}
		nodes := graphNodes(llm, events, &fakeValidator{violations: blocking})
		graph, err := NewGraph(context.Background(), nodes)
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}

		out, err := graph.Run(context.Background(), &TripState{RunID: "run-3", Input: "北京一天"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.StopReason != string(StopNoImprovement) {
			t.Fatalf("stop reason = %s", out.StopReason)
		}
		if len(out.History) != 3 {
			t.Fatalf("history = %+v", out.History)
		}
		if out.Snapshot == nil || len(out.Snapshot.Violations) != 1 {
			t.Fatalf("snapshot = %+v", out.Snapshot)
		}
		if len(out.Suggestions) == 0 {
			t.Fatal("missing relaxation suggestions")
		}
		if out.Snapshot.Reason != "repair" {
			t.Fatalf("snapshot reason = %s", out.Snapshot.Reason)
		}
		if len(llm.calls) != 4 {
			t.Fatalf("llm calls = %d", len(llm.calls))
		}
		types := events.types()
		if countString(types, "repair.started") != 2 || countString(types, "repair.completed") != 2 {
			t.Fatalf("events = %v", types)
		}
	})
}

func TestGraphReplan(t *testing.T) {
	include := false
	previous := graphPlan()

	t.Run("patch fills missing critical fields", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"","days":0,"lodging":{"mentioned":true}}`),
			graphDraftReply(),
		}}
		events := &eventRecorder{}
		nodes := graphNodes(llm, events, &fakeValidator{})
		graph, err := NewGraph(context.Background(), nodes)
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}
		patch := &domain.ConstraintSet{City: "beijing", Days: 1}
		out, err := graph.Run(context.Background(), &TripState{
			RunID: "run-replan-1", Input: "按新的约束重排", Mode: "replan",
			ConstraintPatch: patch, Previous: &previous, PreviousSeq: 2,
		})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.Constraints.City != "beijing" || out.Constraints.Days != 1 {
			t.Fatalf("constraints = %+v", out.Constraints)
		}
		if len(out.Missing) != 0 {
			t.Fatalf("missing = %v, want none after patch", out.Missing)
		}
		if out.Snapshot == nil || out.Snapshot.Seq != 3 || out.Snapshot.Reason != "replan" {
			t.Fatalf("snapshot = %+v", out.Snapshot)
		}
		if out.Previous == nil || len(out.Previous.Days) != 1 {
			t.Fatalf("previous version lost: %+v", out.Previous)
		}
	})

	t.Run("include_lodging false forces lodging off", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1,"lodging":{"mentioned":true,"hotels":["全季"]}}`),
			graphDraftReply(),
		}}
		nodes := graphNodes(llm, &eventRecorder{}, &fakeValidator{})
		graph, err := NewGraph(context.Background(), nodes)
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}
		patch := &domain.ConstraintSet{
			City:    "beijing",
			Days:    1,
			Budget:  domain.BudgetConstraint{IncludeLodging: &include},
			Lodging: domain.LodgingConstraint{Mentioned: false},
		}
		out, err := graph.Run(context.Background(), &TripState{
			RunID: "run-replan-2", Input: "住宿不含在预算里", Mode: "replan",
			ConstraintPatch: patch,
		})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.Constraints.Lodging.Mentioned {
			t.Fatalf("lodging mentioned = true, want explicit false")
		}
		if out.Constraints.Budget.IncludeLodging == nil || *out.Constraints.Budget.IncludeLodging {
			t.Fatalf("include_lodging = %v, want false", out.Constraints.Budget.IncludeLodging)
		}
		if out.Snapshot == nil || out.Snapshot.Reason != "replan" {
			t.Fatalf("snapshot = %+v", out.Snapshot)
		}
	})
}

func TestGraphPlugins(t *testing.T) {
	t.Run("hotel and guide flow through to plan", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":2,"lodging":{"mentioned":true}}`),
			json.RawMessage(`{"days":[{"items":[{"poi_id":"poi-0","type":"poi","duration_min":90}]},{"items":[{"poi_id":"poi-1","type":"poi","duration_min":60}]}]}`),
		}}
		hotelTool := &fakeResearchTool{name: researchHotelTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return hotelToolOutput(t, domain.Hotel{
				ID: "h1", Name: "测试酒店", Lat: 39.80, Lng: 116.30, PricePerNightYuan: 500, Provider: "flyai",
			}), nil
		}}
		guideTool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return guideToolOutput(t, guideResult{
				Title: "北京攻略", URL: "https://example.com/guide", Summary: "推荐 poi-1",
				SiteName: "小红书", Authority: 4, RerankScore: 0.8,
			}), nil
		}}
		toolLLM := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("c1", researchHotelTool, `{"query":"北京酒店"}`)}},
			{ToolCalls: []ToolCall{toolCall("c2", researchGuideTool, `{"query":"北京攻略"}`)}},
			{Content: briefJSON(t, `{"status":"ok","accommodation":{"mentioned":true,"selected":{"id":"h1","name":"测试酒店","lat":39.8,"lng":116.3,"price_per_night_yuan":500,"provider":"flyai"},"anchor_days":[0]},"guides":[{"poi_id":"poi-1","weight":0.7,"mentions":2,"note":"攻略推荐"}]}`)},
		}}
		events := &eventRecorder{}
		nodes := graphNodesWithOptions(llm, events,
			WithToolLLM(toolLLM),
			WithResearchTools(hotelTool, guideTool),
			WithScheduler(scheduler.New()),
			WithValidator(validatorFunc(validator.Validate)),
		)
		graph, err := NewGraph(context.Background(), nodes)
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}
		out, err := graph.Run(context.Background(), &TripState{RunID: "run-plugins", Input: "北京两天，预算800含住宿"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.Research == nil || out.Research.Status != domain.ResearchOK {
			t.Fatalf("research = %+v", out.Research)
		}
		if len(out.Candidates) != 2 {
			t.Fatalf("candidates = %d, want closed set of 2", len(out.Candidates))
		}
		if out.Plan.Lodging == nil {
			t.Fatalf("plan lodging = nil: %+v", out.Plan)
		}
		lodging := out.Plan.Lodging
		if lodging.Nights != 1 || lodging.TotalYuan != 500 || len(lodging.AnchorDays) != 2 || lodging.AnchorDays[0] != 0 || lodging.AnchorDays[1] != 1 {
			t.Fatalf("lodging = %+v", lodging)
		}
		if out.Plan.Totals.LodgingYuan != 500 {
			t.Fatalf("LodgingYuan = %d, want 500", out.Plan.Totals.LodgingYuan)
		}
		day := out.Plan.Days[0]
		if len(day.Blocks) != 3 || day.Blocks[0].ID != "d1-hotel-start" || day.Blocks[2].ID != "d1-hotel-end" {
			t.Fatalf("anchored day blocks = %+v", day.Blocks)
		}
		for _, violation := range out.Violations {
			if violation.Code == "C5" {
				t.Fatalf("unexpected C5: %+v", violation)
			}
		}
		if out.Snapshot == nil || out.Snapshot.Research == nil {
			t.Fatalf("snapshot research = %+v", out.Snapshot)
		}
		if len(llm.calls) < 2 {
			t.Fatalf("llm calls = %d", len(llm.calls))
		}
		draftPrompt := llm.calls[1].prompt
		for _, want := range []string{"guide_score", "distance_to_lodging_km", "anchor_days"} {
			if !strings.Contains(draftPrompt, want) {
				t.Fatalf("draft prompt missing %q:\n%s", want, draftPrompt)
			}
		}
		if !containsString(events.types(), "research.completed") {
			t.Fatalf("events = %v", events.types())
		}
	})

	t.Run("rainy forecast triggers bounded repair", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1}`),
			graphDraftReply(),
			graphRepairReply("poi-0"),
			graphRepairReply("poi-0"),
		}}
		weatherTool := &fakeResearchTool{name: researchWeatherTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return weatherToolOutput(t, domain.WeatherDay{Date: "2026-11-20", DayWeather: "小雨"}), nil
		}}
		toolLLM := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("w1", researchWeatherTool, `{"city":"北京","start_date":"2026-11-20","days":1}`)}},
			{Content: briefJSON(t, `{"status":"ok"}`)},
		}}
		source := &fakeSource{fn: func(context.Context, domain.POIQuery) ([]domain.POI, error) {
			return []domain.POI{{ID: "poi-0", City: "beijing", Name: "长城", Outdoor: true}}, nil
		}}
		events := &eventRecorder{}
		nodes := NewNodes(llm, events,
			WithSnapshotSource(source),
			WithScheduler(&fakeScheduler{plan: graphPlan()}),
			WithValidator(validatorFunc(validator.Validate)),
			WithToolLLM(toolLLM),
			WithResearchTools(weatherTool),
			WithResearchLimits(5*time.Second, 4),
			WithNodeClock(func() time.Time { return time.Date(2026, 11, 19, 10, 0, 0, 0, time.Local) }),
			WithNodeLogger(discardLogger()),
		)
		graph, err := NewGraph(context.Background(), nodes)
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}
		out, err := graph.Run(context.Background(), &TripState{RunID: "run-weather", Input: "北京一天"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.Research == nil || out.Research.Status != domain.ResearchOK || len(out.Research.Weather) != 1 {
			t.Fatalf("research = %+v", out.Research)
		}
		status, days := completedWeather(t, events)
		if status != "ok" || days != 1 {
			t.Fatalf("weather event = %s/%d", status, days)
		}
		if weatherTool.callCount() != 1 {
			t.Fatalf("weather tool calls = %d", weatherTool.callCount())
		}
		if countString(events.types(), "repair.started") != 2 {
			t.Fatalf("events = %v", events.types())
		}
		if out.StopReason != string(StopNoImprovement) || out.Plan.Totals.POICount != 1 {
			t.Fatalf("stop=%s plan=%+v", out.StopReason, out.Plan)
		}
		var sawRain bool
		for _, violation := range out.Snapshot.Violations {
			if violation.Code == "C13" {
				sawRain = true
			}
		}
		if !sawRain {
			t.Fatalf("violations = %+v", out.Snapshot.Violations)
		}
	})

	t.Run("no tools keeps v1 plan shape", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1}`),
			graphDraftReply(),
		}}
		nodes := graphNodesWithOptions(llm, &eventRecorder{},
			WithScheduler(scheduler.New()),
			WithValidator(validatorFunc(validator.Validate)),
		)
		graph, err := NewGraph(context.Background(), nodes)
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}
		out, err := graph.Run(context.Background(), &TripState{RunID: "run-plugins-v1", Input: "北京一天"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.Research == nil || out.Research.Status != domain.ResearchSkipped {
			t.Fatalf("research = %+v", out.Research)
		}
		if out.Plan.Lodging != nil || out.Plan.Totals.LodgingYuan != 0 {
			t.Fatalf("lodging leaked into v1 plan: %+v", out.Plan.Lodging)
		}
		if len(out.Plan.Days) != 1 || len(out.Plan.Days[0].Blocks) != 1 || out.Plan.Totals.POICount != 1 {
			t.Fatalf("v1 plan = %+v", out.Plan)
		}
	})
}

func TestGraphRequiresState(t *testing.T) {
	graph, err := NewGraph(context.Background(), NewNodes(&fakeLLM{}, &eventRecorder{}, WithNodeLogger(discardLogger())))
	if err != nil {
		t.Fatalf("new graph: %v", err)
	}
	if _, err := graph.Run(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil state")
	}
	if _, err := NewGraph(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil nodes")
	}
}

func TestTripStateRoundTrip(t *testing.T) {
	snapshot := VersionSnapshot{
		Seq:          2,
		Plan:         graphPlan(),
		Constraints:  domain.ConstraintSet{City: "beijing", Days: 1},
		Violations:   []domain.Violation{{Code: "C9", Level: domain.LevelWarning, Day: 1, Message: "偏满"}},
		Reason:       "repair",
		Explanations: []domain.Explanation{{POIID: "poi-0", Text: "经典"}},
		Suggestions:  []string{"减少景点"},
		CreatedAt:    time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC),
	}
	best := graphPlan()
	state := &TripState{
		TripID:         "trip-1",
		RunID:          "run-1",
		Mode:           "adjustment",
		Input:          "北京一天，少走路",
		Constraints:    domain.ConstraintSet{City: "beijing", Days: 1},
		Missing:        []string{"days"},
		Candidates:     makePOIs("poi", 2),
		Draft:          domain.DraftPlan{Days: []domain.DraftDay{{Date: "2026-11-20", Items: []domain.DraftItem{{POIID: "poi-0", Type: "poi", DurationMin: 90}}}}},
		Plan:           graphPlan(),
		Violations:     []domain.Violation{{Code: "C4", Level: domain.LevelBlocking, Day: 1, Message: "超时"}},
		Round:          1,
		Best:           &best,
		BestViolations: []domain.Violation{{Code: "C9", Level: domain.LevelWarning, Day: 1, Message: "偏满"}},
		History:        []RoundMetrics{{Round: 0, Blocking: 1, Warnings: 0, CostYuan: 30, WalkingKM: 1.5}},
		StopReason:     "max_rounds",
		Suggestions:    []string{"减少景点"},
		Snapshot:       &snapshot,
		Previous:       &best,
		PreviousSeq:    1,
		Research: &domain.ResearchBrief{
			Status:        domain.ResearchOK,
			Accommodation: &domain.AccommodationBrief{Mentioned: true, AnchorDays: []int{0}},
			Guides:        []domain.GuideSignal{{POIID: "poi-0", Weight: 0.6}},
		},
		ResearchQuestion: &domain.ResearchQuestion{
			Code:    "hotel_unconfirmed",
			Message: "未找到指定酒店",
			Options: []domain.ResearchOption{{ID: "continue_without_lodging", Label: "不带住宿继续"}},
		},
		SkillPins:       []skills.CatalogEntry{{Name: "research-playbook", Version: "1.0.0", Description: "用于研究", SHA256: strings.Repeat("a", 64)}},
		SkillsPinned:    true,
		ResolvedHotel:   &domain.Hotel{ID: "h1", Name: "测试酒店"},
		ConstraintPatch: &domain.ConstraintSet{City: "beijing", Days: 1},
		Status:          StatusAwaitingUser,
		Error:           &RunError{Code: "event_failed", Message: "db down"},
	}

	encoded, err := state.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeTripState(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(state, decoded) {
		t.Fatalf("roundtrip mismatch:\nwant %+v\ngot  %+v", state, decoded)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func countString(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}

func TestGraphPinsSkillCatalogWithoutBodies(t *testing.T) {
	const bodyMarker = "BODY-MARKER-"
	registrySkills := make([]skills.Skill, 0, 10)
	for index := 0; index < 10; index++ {
		registrySkills = append(registrySkills, skills.Skill{
			Name:        "skill-" + string(rune('a'+index)),
			Version:     "1.0.0",
			Description: "用于测试状态体积",
			SHA256:      strings.Repeat("a", 64),
			Body:        bodyMarker + strings.Repeat("x", 10<<10),
		})
	}
	registry := skills.NewRegistry(registrySkills...)
	nodes := NewNodes(nil, &eventRecorder{}, WithSkillRegistry(registry), WithNodeLogger(discardLogger()))
	graph := &Graph{nodes: nodes}

	out, err := graph.admit(context.Background(), &TripState{RunID: "run-1", Input: "北京一天"})
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	encoded, err := out.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(string(encoded), bodyMarker) {
		t.Fatal("run state contains skill bodies")
	}
	if !strings.Contains(string(encoded), `"skill_pins"`) {
		t.Fatalf("run state missing skill_pins: %s", encoded)
	}
	if len(encoded) >= 64<<10 {
		t.Fatalf("run state is %d bytes, expected far below the 100 KiB of bodies", len(encoded))
	}
	if len(out.SkillPins) != len(registrySkills) {
		t.Fatalf("pins = %d", len(out.SkillPins))
	}
}

func TestGraphResearchLoadsLegacySkillBodies(t *testing.T) {
	tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return guideToolOutput(t), nil
	}}
	llm := &fakeToolLLM{responses: []ToolCompletion{
		{ToolCalls: []ToolCall{toolCall("c1", researchSkillTool, `{"name":"research-playbook"}`)}},
		{Content: briefJSON(t, `{"status":"ok"}`)},
	}}
	nodes := NewNodes(nil, &eventRecorder{}, WithToolLLM(llm), WithResearchTools(tool), WithNodeLogger(discardLogger()))
	graph := &Graph{nodes: nodes}
	state := &TripState{
		RunID:        "run-legacy",
		Input:        "北京两天",
		SkillsPinned: true,
		Skills:       []skills.Skill{{Name: "research-playbook", Version: "1.0.0", Description: "用于研究", SHA256: strings.Repeat("a", 64), Body: "LEGACY BODY"}},
		Constraints:  domain.ConstraintSet{City: "beijing", Days: 2},
	}

	out, err := graph.research(context.Background(), state)
	if err != nil {
		t.Fatalf("research: %v", err)
	}
	if out.Research == nil || out.Research.Status != domain.ResearchOK {
		t.Fatalf("research = %+v", out.Research)
	}
	if !strings.Contains(llm.requestAt(t, 1).Messages[2].Content, "LEGACY BODY") {
		t.Fatalf("legacy body not loaded: %s", llm.requestAt(t, 1).Messages[2].Content)
	}
}
