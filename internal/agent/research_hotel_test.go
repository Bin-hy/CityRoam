package agent

import (
	"context"
	"encoding/json"
	"testing"

	"trippilot/internal/domain"
)

func specifiedHotelInput() ResearchInput {
	in := researchTestInput()
	in.Constraints.Days = 2
	in.Constraints.Lodging = domain.LodgingConstraint{Mentioned: true, Hotels: []string{"华尔道夫"}}
	return in
}

func TestResearchHotelGeocode(t *testing.T) {
	t.Run("resolves a specified hotel without research tools", func(t *testing.T) {
		resolver := &fakeHotelResolver{results: map[string][]domain.Hotel{
			"华尔道夫": {{ID: "amap:1", Name: "华尔道夫", Lat: 39.92, Lng: 116.46, Provider: "amap", PricePerNightYuan: 900}},
		}}
		events := &eventRecorder{}
		nodes := researchTestNodes(&fakeLLM{}, events, WithHotelResolver(resolver))

		out, err := nodes.Research(context.Background(), specifiedHotelInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if out.Brief.Status != domain.ResearchOK {
			t.Fatalf("status = %s", out.Brief.Status)
		}
		if out.Question != nil {
			t.Fatalf("question = %+v", out.Question)
		}
		if out.Brief.Accommodation == nil || out.Brief.Accommodation.Selected == nil {
			t.Fatalf("brief = %+v", out.Brief.Accommodation)
		}
		selected := out.Brief.Accommodation.Selected
		if selected.Name != "华尔道夫" || selected.Provider != "amap" {
			t.Fatalf("selected = %+v", selected)
		}
		if len(out.Brief.Accommodation.Options) != 1 {
			t.Fatalf("options = %+v", out.Brief.Accommodation.Options)
		}
		if len(resolver.calls) != 1 {
			t.Fatalf("resolver calls = %v", resolver.calls)
		}
		if !containsString(events.types(), "research.completed") {
			t.Fatalf("events = %v", events.types())
		}
	})

	t.Run("skips geocoding when tools already matched the hotel", func(t *testing.T) {
		hotel := domain.Hotel{ID: "h1", Name: "全季酒店", Lat: 39.91, Lng: 116.47, PricePerNightYuan: 400, Provider: "flyai"}
		tool := &fakeResearchTool{name: researchHotelTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return hotelToolOutput(t, hotel), nil
		}}
		resolver := &fakeHotelResolver{results: map[string][]domain.Hotel{
			"全季酒店": {{ID: "amap:9", Name: "全季酒店", Lat: 39.91, Lng: 116.47, Provider: "amap"}},
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("call-1", researchHotelTool, `{}`)}},
			{Content: briefJSON(t, `{"status":"ok","accommodation":{"mentioned":true,"selected":{"id":"h1","name":"全季酒店","lat":39.91,"lng":116.47,"price_per_night_yuan":400,"provider":"flyai"}}}`)},
		}}
		in := specifiedHotelInput()
		in.Constraints.Lodging = domain.LodgingConstraint{Mentioned: true, Hotels: []string{"全季酒店"}}
		nodes := researchTestNodes(&fakeLLM{}, &eventRecorder{},
			WithToolLLM(llm),
			WithResearchTools(tool),
			WithHotelResolver(resolver),
		)

		out, err := nodes.Research(context.Background(), in)
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if out.Brief.Accommodation == nil || out.Brief.Accommodation.Selected == nil {
			t.Fatalf("brief = %+v", out.Brief.Accommodation)
		}
		if out.Brief.Accommodation.Selected.Provider != "flyai" {
			t.Fatalf("selected = %+v", out.Brief.Accommodation.Selected)
		}
		if len(resolver.calls) != 0 {
			t.Fatalf("resolver calls = %v", resolver.calls)
		}
	})
}
