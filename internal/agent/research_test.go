package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"trippilot/internal/domain"
)

type fakeResearchTool struct {
	name   string
	schema json.RawMessage
	fn     func(ctx context.Context, input json.RawMessage) (json.RawMessage, error)

	mu     sync.Mutex
	calls  int
	inputs []json.RawMessage
}

func (f *fakeResearchTool) Name() string        { return f.name }
func (f *fakeResearchTool) Description() string { return "fake " + f.name }
func (f *fakeResearchTool) InputSchema() json.RawMessage {
	if len(f.schema) == 0 {
		return json.RawMessage(`{"type":"object"}`)
	}
	return f.schema
}

func (f *fakeResearchTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	f.mu.Lock()
	f.calls++
	f.inputs = append(f.inputs, input)
	f.mu.Unlock()
	return f.fn(ctx, input)
}

func (f *fakeResearchTool) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type toolRequest struct {
	System   string
	Messages []ChatMessage
	Tools    []ToolDefinition
}

type fakeToolLLM struct {
	responses []ToolCompletion
	errs      []error
	usage     Usage

	mu       sync.Mutex
	requests []toolRequest
}

func (f *fakeToolLLM) CompleteWithTools(ctx context.Context, req ToolCompletionRequest) (ToolCompletion, Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, toolRequest{System: req.System, Messages: req.Messages, Tools: req.Tools})
	index := len(f.requests) - 1
	if index < len(f.errs) && f.errs[index] != nil {
		return ToolCompletion{}, f.usage, f.errs[index]
	}
	if index < len(f.responses) {
		return f.responses[index], f.usage, nil
	}
	if len(f.responses) == 0 {
		return ToolCompletion{}, f.usage, errors.New("no scripted response")
	}
	return f.responses[len(f.responses)-1], f.usage, nil
}

func (f *fakeToolLLM) requestAt(t *testing.T, index int) toolRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if index >= len(f.requests) {
		t.Fatalf("missing tool request %d (have %d)", index, len(f.requests))
	}
	return f.requests[index]
}

func (f *fakeToolLLM) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func researchTestNodes(base LLMClient, events EventPublisher, opts ...NodeOption) *Nodes {
	options := []NodeOption{
		WithNodeClock(func() time.Time { return time.Date(2026, 11, 19, 10, 0, 0, 0, time.Local) }),
		WithNodeLogger(discardLogger()),
	}
	options = append(options, opts...)
	return NewNodes(base, events, options...)
}

func researchTestInput() ResearchInput {
	return ResearchInput{
		RunID:  "run-research",
		TripID: "trip-research",
		Input:  "北京三天怎么玩",
		Constraints: domain.ConstraintSet{
			City: "beijing",
			Days: 3,
		},
		Candidates: []domain.POI{
			{ID: "poi-0", Name: "故宫", Aliases: []string{"紫禁城"}, City: "beijing"},
			{ID: "poi-1", Name: "天坛", City: "beijing"},
		},
	}
}

func briefJSON(t *testing.T, payload string) string {
	t.Helper()
	if !json.Valid([]byte(payload)) {
		t.Fatalf("brief payload is not JSON: %s", payload)
	}
	return payload
}

func hotelToolOutput(t *testing.T, hotels ...domain.Hotel) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"hotels": hotels})
	if err != nil {
		t.Fatalf("encode hotels: %v", err)
	}
	return payload
}

func guideToolOutput(t *testing.T, results ...guideResult) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"fetched_at": "2026-11-19T00:00:00Z", "results": results})
	if err != nil {
		t.Fatalf("encode guides: %v", err)
	}
	return payload
}

func toolCall(id, name, arguments string) ToolCall {
	return ToolCall{ID: id, Name: name, Arguments: json.RawMessage(arguments)}
}

func TestResearchLoop(t *testing.T) {
	t.Run("executes one tool call and wraps the result", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return guideToolOutput(t, guideResult{
				Title: "故宫攻略", URL: "https://example.com/a", Summary: "故宫很好",
				SiteName: "小红书", Authority: 5, RerankScore: 0.9,
			}), nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("call-1", researchGuideTool, `{"query":"北京故宫"}`)}},
			{Content: briefJSON(t, `{"status":"ok","notes":["done"]}`)},
		}}
		events := &eventRecorder{}
		nodes := researchTestNodes(&fakeLLM{}, events, WithToolLLM(llm), WithResearchTools(tool))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if out.Question != nil {
			t.Fatalf("unexpected question: %+v", out.Question)
		}
		if out.Brief.Status != domain.ResearchOK {
			t.Fatalf("status = %s", out.Brief.Status)
		}
		if tool.callCount() != 1 || out.Brief.ToolCalls != 1 {
			t.Fatalf("tool calls = %d / brief = %d", tool.callCount(), out.Brief.ToolCalls)
		}

		first := llm.requestAt(t, 0)
		if len(first.Tools) != 1 || first.Tools[0].Name != researchGuideTool {
			t.Fatalf("tools = %+v", first.Tools)
		}
		if first.System == "" || len(first.Messages) != 1 || first.Messages[0].Role != "user" {
			t.Fatalf("initial messages = %+v", first.Messages)
		}

		second := llm.requestAt(t, 1)
		var assistant, result *ChatMessage
		for i := range second.Messages {
			message := &second.Messages[i]
			if message.Role == "assistant" && len(message.ToolCalls) == 1 {
				assistant = message
			}
			if message.Role == "tool" {
				result = message
			}
		}
		if assistant == nil || assistant.ToolCalls[0].ID != "call-1" {
			t.Fatalf("assistant message not preserved: %+v", second.Messages)
		}
		if result == nil || result.ToolCallID != "call-1" {
			t.Fatalf("tool result missing tool_call_id: %+v", second.Messages)
		}
		if !strings.Contains(result.Content, "<external_data>") || !strings.Contains(result.Content, "不可信") {
			t.Fatalf("tool result not wrapped as untrusted data: %s", result.Content)
		}

		types := events.types()
		for _, want := range []string{"research.started", "research.completed"} {
			if !containsString(types, want) {
				t.Fatalf("missing event %s in %v", want, types)
			}
		}
		if containsString(types, "research.degraded") {
			t.Fatalf("unexpected degraded event: %v", types)
		}
	})

	t.Run("stops when the model returns no tool calls", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			t.Fatal("tool must not be called")
			return nil, nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{{Content: briefJSON(t, `{"status":"ok"}`)}}}
		nodes := researchTestNodes(nil, &eventRecorder{}, WithToolLLM(llm), WithResearchTools(tool))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if llm.requestCount() != 1 || tool.callCount() != 0 || out.Brief.ToolCalls != 0 {
			t.Fatalf("requests=%d tools=%d tool_calls=%d", llm.requestCount(), tool.callCount(), out.Brief.ToolCalls)
		}
	})

	t.Run("executes multiple calls across turns in order", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchGuideTool, fn: func(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
			return guideToolOutput(t, guideResult{
				Title: string(input), URL: "https://example.com/" + strings.Trim(string(input), `"`),
				RerankScore: 0.4,
			}), nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{
				toolCall("call-1", researchGuideTool, `{"query":"one"}`),
				toolCall("call-2", researchGuideTool, `{"query":"two"}`),
			}},
			{ToolCalls: []ToolCall{toolCall("call-3", researchGuideTool, `{"query":"three"}`)}},
			{Content: briefJSON(t, `{"status":"ok"}`)},
		}}
		nodes := researchTestNodes(nil, &eventRecorder{}, WithToolLLM(llm), WithResearchTools(tool))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if tool.callCount() != 3 || out.Brief.ToolCalls != 3 {
			t.Fatalf("tool calls = %d / brief = %d", tool.callCount(), out.Brief.ToolCalls)
		}
		ids := make([]string, 0, 3)
		for _, message := range llm.requestAt(t, 2).Messages {
			if message.Role == "tool" {
				ids = append(ids, message.ToolCallID)
			}
		}
		if strings.Join(ids, ",") != "call-1,call-2,call-3" {
			t.Fatalf("tool result order = %v", ids)
		}
	})

	t.Run("rejects calls beyond the budget and degrades", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchGuideTool, fn: func(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
			return guideToolOutput(t, guideResult{Title: "note", URL: "https://example.com/n", RerankScore: 0.1}), nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{
				toolCall("c1", researchGuideTool, `{}`),
				toolCall("c2", researchGuideTool, `{}`),
				toolCall("c3", researchGuideTool, `{}`),
			}},
			{ToolCalls: []ToolCall{
				toolCall("c4", researchGuideTool, `{}`),
				toolCall("c5", researchGuideTool, `{}`),
				toolCall("c6", researchGuideTool, `{}`),
			}},
			{ToolCalls: []ToolCall{
				toolCall("c7", researchGuideTool, `{}`),
				toolCall("c8", researchGuideTool, `{}`),
				toolCall("c9", researchGuideTool, `{}`),
			}},
		}}
		events := &eventRecorder{}
		base := &fakeLLM{reply: json.RawMessage(`{"status":"ok","notes":["fallback"]}`)}
		nodes := researchTestNodes(base, events,
			WithToolLLM(llm), WithResearchTools(tool), WithResearchLimits(5*time.Second, 6))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if tool.callCount() != 6 || out.Brief.ToolCalls != 6 {
			t.Fatalf("tool calls = %d / brief = %d", tool.callCount(), out.Brief.ToolCalls)
		}
		if out.Brief.Status != domain.ResearchDegraded {
			t.Fatalf("status = %s", out.Brief.Status)
		}
		if llm.requestCount() != 3 {
			t.Fatalf("tool llm requests = %d", llm.requestCount())
		}
		payload := researchEventPayload(t, events, "research.degraded")
		if payload["reason"] != "tool_call_limit" {
			t.Fatalf("degrade reason = %v", payload["reason"])
		}
	})

	t.Run("times out into a degraded brief instead of an error", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchGuideTool, fn: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("slow", researchGuideTool, `{}`)}},
		}}
		events := &eventRecorder{}
		nodes := researchTestNodes(&fakeLLM{}, events,
			WithToolLLM(llm), WithResearchTools(tool), WithResearchLimits(40*time.Millisecond, 6))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research returned error: %v", err)
		}
		if out.Brief.Status != domain.ResearchDegraded {
			t.Fatalf("status = %s", out.Brief.Status)
		}
		payload := researchEventPayload(t, events, "research.degraded")
		if payload["reason"] != "timeout" {
			t.Fatalf("degrade reason = %v", payload["reason"])
		}
	})

	t.Run("skips entirely without tools", func(t *testing.T) {
		llm := &fakeToolLLM{}
		events := &eventRecorder{}
		nodes := researchTestNodes(nil, events, WithToolLLM(llm))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if out.Brief.Status != domain.ResearchSkipped {
			t.Fatalf("status = %s", out.Brief.Status)
		}
		if llm.requestCount() != 0 {
			t.Fatalf("tool llm calls = %d", llm.requestCount())
		}
		if got := events.types(); len(got) != 0 {
			t.Fatalf("events = %v", got)
		}
	})
}

func TestResearchNormalizeBrief(t *testing.T) {
	hotel := domain.Hotel{ID: "h1", Name: "全季酒店", PricePerNightYuan: 500, Provider: "fake"}
	makeInput := func() ResearchInput {
		in := researchTestInput()
		in.Constraints.Days = 2
		in.Constraints.Lodging = domain.LodgingConstraint{Mentioned: true}
		return in
	}
	selectedBrief := func(days []int) domain.ResearchBrief {
		return domain.ResearchBrief{
			Accommodation: &domain.AccommodationBrief{
				Selected:        &domain.Hotel{ID: "h1"},
				AnchorDays:      days,
				Nights:          99,
				NightlyYuan:     1,
				TotalYuan:       2,
				IncludeInBudget: false,
			},
		}
	}

	t.Run("normalizes anchor days, nights, prices and budget flag", func(t *testing.T) {
		in := makeInput()
		session := &researchSession{hotels: []domain.Hotel{hotel}, hotelAvailable: true}
		brief := selectedBrief([]int{3, -1, 0, 0, 1})
		question := normalizeResearchBrief(&brief, in, session)
		if question != nil {
			t.Fatalf("unexpected question: %+v", question)
		}
		acc := brief.Accommodation
		if acc == nil || acc.Selected == nil || acc.Selected.ID != "h1" {
			t.Fatalf("accommodation = %+v", acc)
		}
		if len(acc.AnchorDays) != 2 || acc.AnchorDays[0] != 0 || acc.AnchorDays[1] != 1 {
			t.Fatalf("anchor days = %v", acc.AnchorDays)
		}
		if acc.Nights != 1 || acc.NightlyYuan != 500 || acc.TotalYuan != 500 {
			t.Fatalf("price = %+v", acc)
		}
		if !acc.IncludeInBudget || !acc.Mentioned || acc.PriceEstimated {
			t.Fatalf("flags = %+v", acc)
		}
	})

	t.Run("uses N-1 nights for the total", func(t *testing.T) {
		in := makeInput()
		in.Constraints.Days = 4
		session := &researchSession{hotels: []domain.Hotel{{ID: "h1", Name: "全季酒店", PricePerNightYuan: 200}}}
		brief := selectedBrief(nil)
		if question := normalizeResearchBrief(&brief, in, session); question != nil {
			t.Fatalf("unexpected question: %+v", question)
		}
		if brief.Accommodation.Nights != 3 || brief.Accommodation.TotalYuan != 600 {
			t.Fatalf("price = %+v", brief.Accommodation)
		}
	})

	t.Run("does not budget an unknown price", func(t *testing.T) {
		in := makeInput()
		session := &researchSession{hotels: []domain.Hotel{{ID: "h1", Name: "全季酒店"}}}
		brief := selectedBrief(nil)
		if question := normalizeResearchBrief(&brief, in, session); question != nil {
			t.Fatalf("unexpected question: %+v", question)
		}
		if brief.Accommodation.NightlyYuan != 0 || brief.Accommodation.TotalYuan != 0 || !brief.Accommodation.PriceEstimated {
			t.Fatalf("price = %+v", brief.Accommodation)
		}
		if !containsString(brief.Warnings, "未获取到房价，住宿费用未计入预算") {
			t.Fatalf("warnings = %v", brief.Warnings)
		}
	})

	t.Run("honours an explicit budget exclusion", func(t *testing.T) {
		in := makeInput()
		excluded := false
		in.Constraints.Budget.IncludeLodging = &excluded
		session := &researchSession{hotels: []domain.Hotel{hotel}}
		brief := selectedBrief(nil)
		normalizeResearchBrief(&brief, in, session)
		if brief.Accommodation.IncludeInBudget {
			t.Fatal("include_in_budget should be false")
		}
	})

	t.Run("cleans fabricated hotels and selection", func(t *testing.T) {
		in := makeInput()
		session := &researchSession{hotels: []domain.Hotel{hotel}}
		brief := domain.ResearchBrief{Accommodation: &domain.AccommodationBrief{
			Selected:   &domain.Hotel{ID: "fake", Name: "编造大酒店", PricePerNightYuan: 100},
			AnchorDays: []int{0},
		}}
		normalizeResearchBrief(&brief, in, session)
		if brief.Accommodation.Selected != nil || brief.Accommodation.AnchorDays != nil {
			t.Fatalf("fabricated selection survived: %+v", brief.Accommodation)
		}
		if !containsString(brief.Warnings, "未确定住宿酒店，按无住宿继续规划") {
			t.Fatalf("warnings = %v", brief.Warnings)
		}
	})

	t.Run("adopts a selected hotel only from tool results", func(t *testing.T) {
		in := makeInput()
		session := &researchSession{hotels: []domain.Hotel{hotel}}
		brief := domain.ResearchBrief{Accommodation: &domain.AccommodationBrief{
			Selected: &domain.Hotel{Name: "全季酒店", PricePerNightYuan: 1},
		}}
		normalizeResearchBrief(&brief, in, session)
		selected := brief.Accommodation.Selected
		if selected == nil || selected.ID != "h1" || selected.PricePerNightYuan != 500 {
			t.Fatalf("selected = %+v", selected)
		}
	})

	t.Run("drops guide signals outside the candidate set and clamps weights", func(t *testing.T) {
		candidates := []domain.POI{{ID: "poi-0", Name: "故宫"}, {ID: "poi-1", Name: "天坛"}}
		longNote := strings.Repeat("热", 200)
		signals := matchGuideSignals(candidates, nil, []domain.GuideSignal{
			{POIID: "poi-9", Weight: 0.5, Note: "不存在"},
			{POIID: "poi-0", Weight: 1.5, Note: longNote, Citations: []domain.GuideCitation{
				{URL: "https://a"}, {URL: "ftp://b"}, {URL: "https://c"}, {URL: "https://d"}, {URL: "https://e"},
			}},
			{POIID: "poi-1", Weight: -0.4, Note: ""},
			{POIID: "poi-0", Weight: 0.2, Note: "duplicate"},
		}, nil)
		if len(signals) != 2 {
			t.Fatalf("signals = %+v", signals)
		}
		first := signals[0]
		if first.POIID != "poi-0" || first.Weight != 1 || len([]rune(first.Note)) != researchNoteLimit {
			t.Fatalf("first signal = %+v", first)
		}
		if len(first.Citations) != 3 {
			t.Fatalf("citations = %+v", first.Citations)
		}
		if signals[1].Weight != 0 || signals[1].Note != "攻略中提及该地点" {
			t.Fatalf("second signal = %+v", signals[1])
		}
	})

	t.Run("degrades when structured fallback fails", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return guideToolOutput(t), nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{{Content: "not a brief"}}}
		events := &eventRecorder{}
		nodes := researchTestNodes(nil, events, WithToolLLM(llm), WithResearchTools(tool))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if out.Brief.Status != domain.ResearchDegraded {
			t.Fatalf("status = %s", out.Brief.Status)
		}
	})
}

func TestResearchSpecifiedHotel(t *testing.T) {
	makeInput := func(specified ...string) ResearchInput {
		in := researchTestInput()
		in.Constraints.Days = 3
		in.Constraints.Lodging = domain.LodgingConstraint{Mentioned: true, Hotels: specified}
		return in
	}

	t.Run("adopts a unique match with a price", func(t *testing.T) {
		in := makeInput("全季")
		session := &researchSession{hotelAvailable: true, hotels: []domain.Hotel{
			{ID: "h1", Name: "全季酒店", PricePerNightYuan: 300},
		}}
		brief := domain.ResearchBrief{Accommodation: &domain.AccommodationBrief{AnchorDays: []int{0}}}
		if question := normalizeResearchBrief(&brief, in, session); question != nil {
			t.Fatalf("unexpected question: %+v", question)
		}
		if brief.Accommodation.Selected == nil || brief.Accommodation.Selected.ID != "h1" {
			t.Fatalf("selected = %+v", brief.Accommodation.Selected)
		}
		if brief.Accommodation.TotalYuan != 600 {
			t.Fatalf("total = %d", brief.Accommodation.TotalYuan)
		}
	})

	t.Run("asks when several hotels match", func(t *testing.T) {
		in := makeInput("全季")
		session := &researchSession{hotelAvailable: true, hotels: []domain.Hotel{
			{ID: "h1", Name: "全季酒店", PricePerNightYuan: 300},
			{ID: "h2", Name: "全季酒店王府井店", PricePerNightYuan: 450},
		}}
		brief := domain.ResearchBrief{}
		question := normalizeResearchBrief(&brief, in, session)
		if question == nil || question.Code != "hotel_ambiguous" {
			t.Fatalf("question = %+v", question)
		}
		if len(question.Options) != 2 {
			t.Fatalf("options = %+v", question.Options)
		}
		if brief.Accommodation.Selected != nil || brief.Accommodation.AnchorDays != nil {
			t.Fatalf("selected should be cleared: %+v", brief.Accommodation)
		}
	})

	t.Run("asks with nearby options when nothing matches", func(t *testing.T) {
		in := makeInput("华尔道夫")
		hotels := make([]domain.Hotel, 0, 7)
		for index := 0; index < 7; index++ {
			hotels = append(hotels, domain.Hotel{ID: "n" + string(rune('0'+index)), Name: "附近酒店" + string(rune('0'+index)), PricePerNightYuan: 200})
		}
		session := &researchSession{hotelAvailable: true, hotels: hotels}
		brief := domain.ResearchBrief{}
		question := normalizeResearchBrief(&brief, in, session)
		if question == nil || question.Code != "hotel_unconfirmed" {
			t.Fatalf("question = %+v", question)
		}
		if !hasOption(question, "retry_search") || !hasOption(question, "continue_without_lodging") {
			t.Fatalf("missing retry options: %+v", question.Options)
		}
		hotelOptions := 0
		for _, option := range question.Options {
			if option.Hotel != nil {
				hotelOptions++
			}
		}
		if hotelOptions != researchOptionsMax {
			t.Fatalf("hotel options = %d", hotelOptions)
		}
		for _, option := range question.Options {
			if option.ID == "continue_without_lodging" && !strings.Contains(option.Label, "不计住宿费用") {
				t.Fatalf("label = %s", option.Label)
			}
		}
	})

	t.Run("asks when the matched hotel has no price", func(t *testing.T) {
		in := makeInput("全季")
		session := &researchSession{hotelAvailable: true, hotels: []domain.Hotel{
			{ID: "h1", Name: "全季酒店"},
		}}
		brief := domain.ResearchBrief{}
		question := normalizeResearchBrief(&brief, in, session)
		if question == nil || question.Code != "hotel_unconfirmed" {
			t.Fatalf("question = %+v", question)
		}
		if brief.Accommodation.Selected != nil {
			t.Fatalf("selected = %+v", brief.Accommodation.Selected)
		}
		if !containsString(brief.Warnings, "未获取到房价，住宿费用未计入预算") {
			t.Fatalf("warnings = %v", brief.Warnings)
		}
	})

	t.Run("reports plugin unavailability but keeps the ask", func(t *testing.T) {
		in := makeInput("全季")
		session := &researchSession{hotelAvailable: false}
		brief := domain.ResearchBrief{}
		question := normalizeResearchBrief(&brief, in, session)
		if question == nil || question.Code != "hotel_plugin_unavailable" {
			t.Fatalf("question = %+v", question)
		}
		if !hasOption(question, "retry_search") || !hasOption(question, "continue_without_lodging") {
			t.Fatalf("options = %+v", question.Options)
		}
	})

	t.Run("adopts a resolved hotel without asking", func(t *testing.T) {
		in := makeInput()
		in.ResolvedHotel = &domain.Hotel{ID: "rh", Name: "自选酒店", PricePerNightYuan: 500}
		session := &researchSession{hotelAvailable: true}
		brief := domain.ResearchBrief{}
		if question := normalizeResearchBrief(&brief, in, session); question != nil {
			t.Fatalf("unexpected question: %+v", question)
		}
		if brief.Accommodation.Selected == nil || brief.Accommodation.Selected.ID != "rh" {
			t.Fatalf("selected = %+v", brief.Accommodation.Selected)
		}
		if brief.Accommodation.TotalYuan != 1000 {
			t.Fatalf("total = %d", brief.Accommodation.TotalYuan)
		}
	})
}

func TestResearchMatchGuides(t *testing.T) {
	candidates := []domain.POI{
		{ID: "poi-0", Name: "故宫", Aliases: []string{"紫禁城"}},
		{ID: "poi-1", Name: "天坛"},
	}

	t.Run("matches names and aliases once per result", func(t *testing.T) {
		signals := matchGuideSignals(candidates, []guideResult{{
			Title: "紫禁城攻略", URL: "https://example.com/a", Snippet: "故宫和紫禁城都值得去",
			Authority: 5, RerankScore: 0.8,
		}}, nil, nil)
		if len(signals) != 1 || signals[0].POIID != "poi-0" {
			t.Fatalf("signals = %+v", signals)
		}
		if signals[0].Mentions != 1 {
			t.Fatalf("mentions = %d", signals[0].Mentions)
		}
		want := 0.45 + 0.12 + 0.02*5 + 0.05*0.8
		if diff := signals[0].Weight - want; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("weight = %f, want %f", signals[0].Weight, want)
		}
		if signals[0].Note != "攻略中提及该地点" || len(signals[0].Citations) != 1 {
			t.Fatalf("signal = %+v", signals[0])
		}
	})

	t.Run("keeps the top three citations by rerank", func(t *testing.T) {
		results := []guideResult{
			{Title: "a", URL: "https://a", Summary: "故宫", RerankScore: 0.2},
			{Title: "b", URL: "https://b", Summary: "故宫", RerankScore: 0.9},
			{Title: "c", URL: "https://c", Summary: "故宫", RerankScore: 0.5},
			{Title: "d", URL: "https://d", Summary: "故宫", RerankScore: 0.7},
			{Title: "e", URL: "ftp://e", Summary: "故宫", RerankScore: 1},
		}
		signals := matchGuideSignals(candidates, results, nil, nil)
		if len(signals) != 1 {
			t.Fatalf("signals = %+v", signals)
		}
		if signals[0].Mentions != 5 {
			t.Fatalf("mentions = %d", signals[0].Mentions)
		}
		urls := make([]string, 0, len(signals[0].Citations))
		for _, citation := range signals[0].Citations {
			urls = append(urls, citation.URL)
		}
		if strings.Join(urls, ",") != "https://b,https://d,https://c" {
			t.Fatalf("citations = %v", urls)
		}
	})

	t.Run("records unmatched notes without adding POIs", func(t *testing.T) {
		var notes []string
		signals := matchGuideSignals(candidates, []guideResult{
			{Title: "北京烤鸭推荐", URL: "https://food", Snippet: "全聚德", RerankScore: 0.3},
		}, nil, &notes)
		if len(signals) != 0 {
			t.Fatalf("signals = %+v", signals)
		}
		if len(notes) != 1 || !strings.Contains(notes[0], "参考但未对应具体候选") {
			t.Fatalf("notes = %v", notes)
		}
	})

	t.Run("keeps hotels when a later tool fails", func(t *testing.T) {
		hotelTool := &fakeResearchTool{name: researchHotelTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return hotelToolOutput(t, domain.Hotel{ID: "h1", Name: "全季酒店", PricePerNightYuan: 300}), nil
		}}
		guideTool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("upstream down")
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{
				toolCall("h", researchHotelTool, `{}`),
				toolCall("g", researchGuideTool, `{}`),
			}},
			{Content: briefJSON(t, `{"status":"ok","accommodation":{"selected":{"id":"h1"}}}`)},
		}}
		in := researchTestInput()
		in.Constraints.Lodging = domain.LodgingConstraint{Mentioned: true}
		nodes := researchTestNodes(nil, &eventRecorder{}, WithToolLLM(llm), WithResearchTools(hotelTool, guideTool))

		out, err := nodes.Research(context.Background(), in)
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if out.Brief.Accommodation == nil || out.Brief.Accommodation.Selected == nil {
			t.Fatalf("hotel selection lost: %+v", out.Brief.Accommodation)
		}
		if out.Brief.Accommodation.Selected.ID != "h1" {
			t.Fatalf("selected = %+v", out.Brief.Accommodation.Selected)
		}
		if out.Brief.Status != domain.ResearchDegraded {
			t.Fatalf("status = %s", out.Brief.Status)
		}
	})
}

func TestResearchStructuredRetry(t *testing.T) {
	hotelTool := &fakeResearchTool{name: researchHotelTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return hotelToolOutput(t, domain.Hotel{ID: "h1", Name: "全季酒店", PricePerNightYuan: 400}), nil
	}}
	llm := &fakeToolLLM{responses: []ToolCompletion{
		{ToolCalls: []ToolCall{toolCall("h", researchHotelTool, `{}`)}},
		{Content: briefJSON(t, `{"status":"ok"}`)},
	}}
	base := &fakeLLM{reply: json.RawMessage(`{"status":"ok","accommodation":{"selected":{"name":"全季酒店"}}}`)}
	in := researchTestInput()
	in.Constraints.Lodging = domain.LodgingConstraint{Mentioned: true}

	nodes := researchTestNodes(base, &eventRecorder{}, WithToolLLM(llm), WithResearchTools(hotelTool))
	out, err := nodes.Research(context.Background(), in)
	if err != nil {
		t.Fatalf("research: %v", err)
	}
	if len(base.calls) != 1 {
		t.Fatalf("fallback calls = %d", len(base.calls))
	}
	if out.Brief.Accommodation == nil || out.Brief.Accommodation.Selected == nil {
		t.Fatalf("fallback selection missing: %+v", out.Brief.Accommodation)
	}
	if out.Brief.Accommodation.Selected.ID != "h1" {
		t.Fatalf("selected = %+v", out.Brief.Accommodation.Selected)
	}
	if out.Question != nil || out.Brief.Status != domain.ResearchOK {
		t.Fatalf("question=%+v status=%s", out.Question, out.Brief.Status)
	}
}

func TestGraphResearch(t *testing.T) {
	t.Run("runs the node without tools", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1}`),
			graphDraftReply(),
		}}
		events := &eventRecorder{}
		nodes := graphNodes(llm, events, &fakeValidator{})
		recorder := &stageRecorder{}
		graph, err := NewGraph(context.Background(), nodes, WithStageHook(recorder.hook))
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}

		out, err := graph.Run(context.Background(), &TripState{RunID: "run-research", Input: "北京一天"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.Research == nil || out.Research.Status != domain.ResearchSkipped {
			t.Fatalf("research = %+v", out.Research)
		}
		if out.Snapshot == nil || out.Snapshot.Seq != 1 || out.Status != StatusAwaitingUser {
			t.Fatalf("state = %+v", out)
		}
		if got := recorder.stages(); strings.Join(got, ",") != "admit,parse,retrieve,research,draft,schedule,validate,present" {
			t.Fatalf("stages = %v", got)
		}
	})

	t.Run("runs tools then drafts", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1}`),
			graphDraftReply(),
		}}
		tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return guideToolOutput(t), nil
		}}
		toolLLM := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("c1", researchGuideTool, `{}`)}},
			{Content: briefJSON(t, `{"status":"ok"}`)},
		}}
		events := &eventRecorder{}
		nodes := graphNodesWithOptions(llm, events, WithToolLLM(toolLLM), WithResearchTools(tool))
		recorder := &stageRecorder{}
		graph, err := NewGraph(context.Background(), nodes, WithStageHook(recorder.hook))
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}

		out, err := graph.Run(context.Background(), &TripState{RunID: "run-research", Input: "北京一天"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.Research == nil || out.Research.Status != domain.ResearchOK {
			t.Fatalf("research = %+v", out.Research)
		}
		if out.Research.ToolCalls != 1 || tool.callCount() != 1 {
			t.Fatalf("tool calls = %d / %d", out.Research.ToolCalls, tool.callCount())
		}
		if out.Snapshot == nil {
			t.Fatal("draft stage did not run")
		}
		if !containsString(recorder.stages(), "draft") {
			t.Fatalf("stages = %v", recorder.stages())
		}
	})

	t.Run("stops for an unconfirmed hotel and persists the question", func(t *testing.T) {
		llm := &fakeLLM{reply: json.RawMessage(`{"city":"北京","days":2,"lodging":{"mentioned":true,"hotels":["华尔道夫"]}}`)}
		tool := &fakeResearchTool{name: researchHotelTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return hotelToolOutput(t, domain.Hotel{ID: "h1", Name: "全季酒店", PricePerNightYuan: 300}), nil
		}}
		toolLLM := &fakeToolLLM{responses: []ToolCompletion{{Content: briefJSON(t, `{"status":"ok"}`)}}}
		events := &eventRecorder{}
		nodes := graphNodesWithOptions(llm, events, WithToolLLM(toolLLM), WithResearchTools(tool))
		recorder := &stageRecorder{}
		graph, err := NewGraph(context.Background(), nodes, WithStageHook(recorder.hook))
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}

		out, err := graph.Run(context.Background(), &TripState{RunID: "run-research", Input: "北京两天，住华尔道夫"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.ResearchQuestion == nil || out.ResearchQuestion.Code != "hotel_unconfirmed" {
			t.Fatalf("question = %+v", out.ResearchQuestion)
		}
		if out.Status != StatusAwaitingUser || out.Snapshot != nil {
			t.Fatalf("state = %+v", out)
		}
		if got := recorder.stages(); got[len(got)-1] != "research" {
			t.Fatalf("stages = %v", got)
		}
		if !recorder.questionSeen("research") {
			t.Fatal("stage hook did not persist the research question")
		}
	})

	t.Run("does not touch tool network when plugins are disabled", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1}`),
			graphDraftReply(),
		}}
		toolLLM := &fakeToolLLM{}
		nodes := graphNodesWithOptions(llm, &eventRecorder{}, WithToolLLM(toolLLM))
		graph, err := NewGraph(context.Background(), nodes)
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}
		out, err := graph.Run(context.Background(), &TripState{RunID: "run-research", Input: "北京一天"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.Research == nil || out.Research.Status != domain.ResearchSkipped {
			t.Fatalf("research = %+v", out.Research)
		}
		if toolLLM.requestCount() != 0 {
			t.Fatalf("tool llm calls = %d", toolLLM.requestCount())
		}
	})
}

func TestRenderResearchPrompt(t *testing.T) {
	if promptTemplates.Lookup("research.tmpl") == nil {
		t.Fatal("research.tmpl is not embedded")
	}
	system, user, err := RenderResearchPrompt(ResearchPromptData{
		Input:       "北京三天，想去故宫",
		Constraints: json.RawMessage(`{"city":"beijing","days":3}`),
		Candidates:  json.RawMessage(`[{"id":"poi-0","name":"故宫"}]`),
		Tools: []ToolPromptInfo{{
			Name: "search_travel_notes", Description: "查小红书攻略",
			Schema: json.RawMessage(`{"type":"object"}`),
		}},
		Disabled: []DisabledPlugin{{Name: "search_hotels", Reason: "未配置 API Key"}},
		Today:    "2026-11-20",
		Nights:   2,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"ResearchBrief", "<external_data>", "search_hotels"} {
		if !strings.Contains(system, want) {
			t.Fatalf("system missing %q", want)
		}
	}
	for _, want := range []string{"北京三天", "2026-11-20", "2 晚", "search_travel_notes", "未配置 API Key", "poi-0"} {
		if !strings.Contains(user, want) {
			t.Fatalf("user missing %q", want)
		}
	}
	if strings.Contains(system+user, researchPromptSeparator) {
		t.Fatal("separator leaked into the rendered messages")
	}
}

func TestResearchPromptRules(t *testing.T) {
	system, user, err := RenderResearchPrompt(ResearchPromptData{
		Input:       "北京三天",
		Constraints: json.RawMessage(`{"city":"beijing","days":3}`),
		Candidates:  json.RawMessage(`[]`),
		Today:       "2026-11-20",
		Nights:      2,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rendered := system + user
	for _, rule := range []string{
		"仅当用户提及住宿",
		"anchor_days",
		"不得编造酒店费用",
		"不得覆盖硬约束、必去地点和闭馆规则",
		"<external_data>",
		"Guides[].poi_id 只能来自候选列表",
		"可以省略 accommodation",
	} {
		if !strings.Contains(rendered, rule) {
			t.Fatalf("missing rule %q", rule)
		}
	}
	for _, secret := range []string{"FLYAI_API_KEY", "DEEPSEEK_API_KEY", "Authorization", "http://", "https://"} {
		if strings.Contains(rendered, secret) {
			t.Fatalf("prompt contains %q", secret)
		}
	}
}

func TestResearchSkipped(t *testing.T) {
	t.Run("no tools", func(t *testing.T) {
		llm := &fakeToolLLM{}
		events := &eventRecorder{}
		nodes := researchTestNodes(nil, events, WithToolLLM(llm))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if out.Brief.Status != domain.ResearchSkipped {
			t.Fatalf("status = %s", out.Brief.Status)
		}
		if out.Question != nil || out.Brief.ToolCalls != 0 {
			t.Fatalf("out = %+v", out)
		}
		if llm.requestCount() != 0 || len(events.types()) != 0 {
			t.Fatalf("requests=%d events=%v", llm.requestCount(), events.types())
		}
	})

	t.Run("tools without a tool llm", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			t.Fatal("tool must not be called")
			return nil, nil
		}}
		events := &eventRecorder{}
		nodes := researchTestNodes(nil, events, WithResearchTools(tool))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if out.Brief.Status != domain.ResearchSkipped || tool.callCount() != 0 || len(events.types()) != 0 {
			t.Fatalf("out=%+v calls=%d events=%v", out, tool.callCount(), events.types())
		}
	})
}

func TestResearchLoopMessages(t *testing.T) {
	tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return guideToolOutput(t, guideResult{Title: "攻略", URL: "https://example.com/a", RerankScore: 0.3}), nil
	}}
	llm := &fakeToolLLM{responses: []ToolCompletion{
		{ToolCalls: []ToolCall{toolCall("call-9", researchGuideTool, `{"query":"北京"}`)}},
		{Content: briefJSON(t, `{"status":"ok"}`)},
	}}
	nodes := researchTestNodes(nil, &eventRecorder{}, WithToolLLM(llm), WithResearchTools(tool))
	if _, err := nodes.Research(context.Background(), researchTestInput()); err != nil {
		t.Fatalf("research: %v", err)
	}

	second := llm.requestAt(t, 1)
	if second.System == "" || !strings.Contains(second.System, "ResearchBrief") {
		t.Fatalf("system prompt = %q", second.System)
	}
	if len(second.Tools) != 1 {
		t.Fatalf("tools = %+v", second.Tools)
	}
	if string(second.Tools[0].Parameters) != `{"type":"object"}` {
		t.Fatalf("parameters = %s", second.Tools[0].Parameters)
	}
	if second.Messages[0].Role != "user" {
		t.Fatalf("first message = %+v", second.Messages[0])
	}
	assistant, result := false, false
	for _, message := range second.Messages {
		if message.Role == "assistant" && len(message.ToolCalls) == 1 && message.ToolCalls[0].ID == "call-9" {
			assistant = true
		}
		if message.Role == "tool" && message.ToolCallID == "call-9" &&
			strings.HasPrefix(message.Content, "<external_data>") {
			result = true
		}
	}
	if !assistant || !result {
		t.Fatalf("messages = %+v", second.Messages)
	}
}

func TestResearchToolResults(t *testing.T) {
	hotelTool := &fakeResearchTool{name: researchHotelTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return hotelToolOutput(t, domain.Hotel{ID: "h1", Name: "全季酒店", PricePerNightYuan: 300}), nil
	}}
	guideTool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return guideToolOutput(t, guideResult{Title: "攻略", URL: "https://example.com/a"}), nil
	}}
	llm := &fakeToolLLM{responses: []ToolCompletion{
		{ToolCalls: []ToolCall{
			toolCall("u1", "made_up", `{}`),
			toolCall("h1", researchHotelTool, `{}`),
			toolCall("g1", researchGuideTool, `{}`),
		}},
		{Content: briefJSON(t, `{"status":"ok"}`)},
	}}
	nodes := researchTestNodes(nil, &eventRecorder{}, WithToolLLM(llm), WithResearchTools(hotelTool, guideTool))
	if _, err := nodes.Research(context.Background(), researchTestInput()); err != nil {
		t.Fatalf("research: %v", err)
	}

	if hotelTool.callCount() != 0 {
		t.Fatalf("hotel tool called despite no lodging: %d", hotelTool.callCount())
	}
	if guideTool.callCount() != 1 {
		t.Fatalf("guide tool calls = %d", guideTool.callCount())
	}
	var contents []string
	for _, message := range llm.requestAt(t, 1).Messages {
		if message.Role == "tool" {
			contents = append(contents, message.Content)
		}
	}
	if len(contents) != 3 {
		t.Fatalf("tool messages = %v", contents)
	}
	if !strings.Contains(contents[0], "unknown_tool") {
		t.Fatalf("unknown tool result = %s", contents[0])
	}
	if !strings.Contains(contents[1], "hotel_not_mentioned") {
		t.Fatalf("hotel gate result = %s", contents[1])
	}
	if !strings.Contains(contents[2], "<external_data>") {
		t.Fatalf("guide result = %s", contents[2])
	}
}

func TestResearchToolBudget(t *testing.T) {
	tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return guideToolOutput(t, guideResult{Title: "攻略", URL: "https://example.com/a"}), nil
	}}
	llm := &fakeToolLLM{responses: []ToolCompletion{
		{ToolCalls: []ToolCall{
			toolCall("c1", researchGuideTool, `{}`),
			toolCall("c2", researchGuideTool, `{}`),
			toolCall("c3", researchGuideTool, `{}`),
		}},
	}}
	events := &eventRecorder{}
	base := &fakeLLM{reply: json.RawMessage(`{"status":"ok"}`)}
	nodes := researchTestNodes(base, events,
		WithToolLLM(llm), WithResearchTools(tool), WithResearchLimits(5*time.Second, 2))

	out, err := nodes.Research(context.Background(), researchTestInput())
	if err != nil {
		t.Fatalf("research: %v", err)
	}
	if tool.callCount() != 2 || out.Brief.ToolCalls != 2 {
		t.Fatalf("tool calls = %d / %d", tool.callCount(), out.Brief.ToolCalls)
	}
	if llm.requestCount() != 1 {
		t.Fatalf("tool llm requests = %d", llm.requestCount())
	}
	if out.Brief.Status != domain.ResearchDegraded {
		t.Fatalf("status = %s", out.Brief.Status)
	}
	if reason := researchEventPayload(t, events, "research.degraded")["reason"]; reason != "tool_call_limit" {
		t.Fatalf("reason = %v", reason)
	}
}

func TestResearchBriefParse(t *testing.T) {
	guideTool := func() *fakeResearchTool {
		return &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return guideToolOutput(t), nil
		}}
	}

	t.Run("accepts fenced JSON", func(t *testing.T) {
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("c1", researchGuideTool, `{}`)}},
			{Content: "```json\n{\"status\":\"ok\"}\n```"},
		}}
		base := &fakeLLM{reply: json.RawMessage(`{"status":"ok"}`)}
		nodes := researchTestNodes(base, &eventRecorder{}, WithToolLLM(llm), WithResearchTools(guideTool()))
		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if out.Brief.Status != domain.ResearchOK || len(base.calls) != 0 {
			t.Fatalf("status=%s fallback calls=%d", out.Brief.Status, len(base.calls))
		}
	})

	t.Run("falls back once to structured output", func(t *testing.T) {
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("c1", researchGuideTool, `{}`)}},
			{Content: "not json"},
		}}
		base := &fakeLLM{reply: json.RawMessage(`{"status":"ok","notes":["fallback"]}`)}
		nodes := researchTestNodes(base, &eventRecorder{}, WithToolLLM(llm), WithResearchTools(guideTool()))
		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if len(base.calls) != 1 {
			t.Fatalf("fallback calls = %d", len(base.calls))
		}
		if out.Brief.Status != domain.ResearchOK || !containsString(out.Brief.Notes, "fallback") {
			t.Fatalf("brief = %+v", out.Brief)
		}
	})

	t.Run("degrades when fallback is unusable", func(t *testing.T) {
		llm := &fakeToolLLM{responses: []ToolCompletion{{Content: "not json"}}}
		base := &fakeLLM{reply: json.RawMessage(`still not json`)}
		events := &eventRecorder{}
		nodes := researchTestNodes(base, events, WithToolLLM(llm), WithResearchTools(guideTool()))
		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if out.Brief.Status != domain.ResearchDegraded || out.Brief.ToolCalls != 0 {
			t.Fatalf("brief = %+v", out.Brief)
		}
		if reason := researchEventPayload(t, events, "research.degraded")["reason"]; reason != "brief_fallback_invalid" {
			t.Fatalf("reason = %v", reason)
		}
	})
}

func TestResearchQuestion(t *testing.T) {
	t.Run("unconfirmed hotel offers retry and continue", func(t *testing.T) {
		in := researchTestInput()
		in.Constraints.Lodging = domain.LodgingConstraint{Mentioned: true, Hotels: []string{"华尔道夫"}}
		session := &researchSession{hotelAvailable: true, hotels: []domain.Hotel{
			{ID: "h1", Name: "全季酒店", PricePerNightYuan: 300},
		}}
		brief := domain.ResearchBrief{}
		question := normalizeResearchBrief(&brief, in, session)
		if question == nil || question.Code != "hotel_unconfirmed" {
			t.Fatalf("question = %+v", question)
		}
		if !hasOption(question, "retry_search") || !hasOption(question, "continue_without_lodging") {
			t.Fatalf("options = %+v", question.Options)
		}
		if len(question.Options) != 3 {
			t.Fatalf("options = %+v", question.Options)
		}
		if brief.Status != domain.ResearchDegraded {
			t.Fatalf("status = %s", brief.Status)
		}
	})

	t.Run("ambiguous hotel asks for a choice", func(t *testing.T) {
		in := researchTestInput()
		in.Constraints.Lodging = domain.LodgingConstraint{Mentioned: true, Hotels: []string{"全季"}}
		session := &researchSession{hotelAvailable: true, hotels: []domain.Hotel{
			{ID: "h1", Name: "全季酒店", PricePerNightYuan: 300},
			{ID: "h2", Name: "全季酒店王府井店", PricePerNightYuan: 400},
		}}
		brief := domain.ResearchBrief{}
		question := normalizeResearchBrief(&brief, in, session)
		if question == nil || question.Code != "hotel_ambiguous" {
			t.Fatalf("question = %+v", question)
		}
		if hasOption(question, "retry_search") {
			t.Fatalf("ambiguous should only offer hotels: %+v", question.Options)
		}
	})
}

func TestMatchGuides(t *testing.T) {
	candidates := []domain.POI{{ID: "poi-0", Name: "故宫", Aliases: []string{"紫禁城"}}, {ID: "poi-1", Name: "天坛"}}
	var notes []string
	signals := matchGuideSignals(candidates, []guideResult{
		{Title: "紫禁城一日游", URL: "https://example.com/a", Snippet: "故宫", Authority: 3, RerankScore: 0.6},
		{Title: "烤鸭推荐", URL: "https://example.com/b", Snippet: "全聚德"},
	}, nil, &notes)
	if len(signals) != 1 || signals[0].POIID != "poi-0" {
		t.Fatalf("signals = %+v", signals)
	}
	if signals[0].Mentions != 1 || len(signals[0].Citations) != 1 {
		t.Fatalf("signal = %+v", signals[0])
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %v", notes)
	}
}

func TestResearchDegradeEvents(t *testing.T) {
	t.Run("publishes started and completed on success", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return guideToolOutput(t, guideResult{Title: "攻略", URL: "https://example.com/a"}), nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("c1", researchGuideTool, `{}`)}},
			{Content: briefJSON(t, `{"status":"ok"}`)},
		}}
		events := &eventRecorder{}
		nodes := researchTestNodes(nil, events, WithToolLLM(llm), WithResearchTools(tool))
		if _, err := nodes.Research(context.Background(), researchTestInput()); err != nil {
			t.Fatalf("research: %v", err)
		}
		types := events.types()
		if len(types) != 2 || types[0] != "research.started" || types[1] != "research.completed" {
			t.Fatalf("events = %v", types)
		}
	})

	t.Run("publishes degraded and needs_input for an unconfirmed hotel", func(t *testing.T) {
		hotelTool := &fakeResearchTool{name: researchHotelTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return hotelToolOutput(t, domain.Hotel{ID: "h1", Name: "全季酒店", PricePerNightYuan: 300}), nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("h1", researchHotelTool, `{}`)}},
			{Content: briefJSON(t, `{"status":"ok"}`)},
		}}
		in := researchTestInput()
		in.Constraints.Days = 2
		in.Constraints.Lodging = domain.LodgingConstraint{Mentioned: true, Hotels: []string{"华尔道夫"}}
		events := &eventRecorder{}
		nodes := researchTestNodes(nil, events, WithToolLLM(llm), WithResearchTools(hotelTool))

		out, err := nodes.Research(context.Background(), in)
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if out.Question == nil || out.Brief.Status != domain.ResearchDegraded {
			t.Fatalf("out = %+v", out)
		}
		if out.Brief.Accommodation == nil || out.Brief.Accommodation.Selected != nil {
			t.Fatalf("accommodation = %+v", out.Brief.Accommodation)
		}
		types := events.types()
		for _, want := range []string{"research.started", "research.completed", "research.degraded", "research.needs_input"} {
			if !containsString(types, want) {
				t.Fatalf("missing %s in %v", want, types)
			}
		}
	})
}

func TestResearchOrchestration(t *testing.T) {
	tool := &fakeResearchTool{name: researchGuideTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return guideToolOutput(t, guideResult{Title: "攻略", URL: "https://example.com/a"}), nil
	}}
	toolUsage := Usage{Model: "tool-model", TotalTokens: 10, Calls: 1}
	llm := &fakeToolLLM{usage: toolUsage, responses: []ToolCompletion{
		{ToolCalls: []ToolCall{toolCall("c1", researchGuideTool, `{}`)}},
		{Content: "not a brief"},
	}}
	base := &fakeLLM{usage: Usage{Model: "json-model", TotalTokens: 5, Calls: 1}, reply: json.RawMessage(`{"status":"ok"}`)}
	events := &eventRecorder{}
	nodes := researchTestNodes(base, events, WithToolLLM(llm), WithResearchTools(tool))

	in := researchTestInput()
	before := append([]domain.POI(nil), in.Candidates...)
	out, err := nodes.Research(context.Background(), in)
	if err != nil {
		t.Fatalf("research: %v", err)
	}
	if out.Usage.TotalTokens != 25 || out.Usage.Calls != 3 {
		t.Fatalf("usage = %+v", out.Usage)
	}
	if out.Brief.ToolCalls != 1 || out.Brief.DurationMS < 0 {
		t.Fatalf("brief = %+v", out.Brief)
	}
	if !reflect.DeepEqual(before, in.Candidates) {
		t.Fatal("research mutated the candidate set")
	}
	types := events.types()
	if types[0] != "research.started" || types[len(types)-1] != "research.completed" {
		t.Fatalf("events = %v", types)
	}
}

func graphNodesWithOptions(llm LLMClient, events *eventRecorder, opts ...NodeOption) *Nodes {
	source := &fakeSource{fn: func(context.Context, domain.POIQuery) ([]domain.POI, error) {
		return makePOIs("poi", 2), nil
	}}
	base := []NodeOption{
		WithSnapshotSource(source),
		WithScheduler(&fakeScheduler{plan: graphPlan()}),
		WithValidator(&fakeValidator{}),
		WithNodeClock(func() time.Time { return time.Date(2026, 11, 19, 10, 0, 0, 0, time.Local) }),
		WithNodeLogger(discardLogger()),
	}
	return NewNodes(llm, events, append(base, opts...)...)
}

type stageRecorder struct {
	mu     sync.Mutex
	record []string
	withQ  map[string]bool
}

func (r *stageRecorder) hook(_ context.Context, stage string, state *TripState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.record = append(r.record, stage)
	if r.withQ == nil {
		r.withQ = make(map[string]bool)
	}
	r.withQ[stage] = state.ResearchQuestion != nil
	return nil
}

func (r *stageRecorder) stages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.record...)
}

func (r *stageRecorder) questionSeen(stage string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.withQ[stage]
}

func researchEventPayload(t *testing.T, events *eventRecorder, typ string) map[string]any {
	t.Helper()
	events.mu.Lock()
	defer events.mu.Unlock()
	for _, event := range events.events {
		if event.typ != typ {
			continue
		}
		payload, _ := event.payload.(map[string]any)
		if payload == nil {
			t.Fatalf("event %s payload has type %T", typ, event.payload)
		}
		return payload
	}
	t.Fatalf("missing event %s in %v", typ, eventTypesLocked(events))
	return nil
}

func eventTypesLocked(events *eventRecorder) []string {
	out := make([]string, 0, len(events.events))
	for _, event := range events.events {
		out = append(out, event.typ)
	}
	return out
}

func hasOption(question *domain.ResearchQuestion, id string) bool {
	for _, option := range question.Options {
		if option.ID == id {
			return true
		}
	}
	return false
}
