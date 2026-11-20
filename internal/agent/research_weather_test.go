package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"trippilot/internal/domain"
)

func weatherToolOutput(t *testing.T, days ...domain.WeatherDay) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"provider":   "amap",
		"city":       "北京",
		"days":       days,
		"fetched_at": "2026-11-19T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("encode weather: %v", err)
	}
	return payload
}

func completedWeather(t *testing.T, events *eventRecorder) (string, int) {
	t.Helper()
	events.mu.Lock()
	defer events.mu.Unlock()
	for _, event := range events.events {
		if event.typ != "research.completed" {
			continue
		}
		payload, ok := event.payload.(map[string]any)
		if !ok {
			t.Fatalf("completed payload = %T", event.payload)
		}
		status, _ := payload["weather_status"].(string)
		days, _ := payload["weather_days"].(int)
		return status, days
	}
	t.Fatal("missing research.completed event")
	return "", 0
}

func TestResearchWeather(t *testing.T) {
	forecast := []domain.WeatherDay{
		{Date: "2026-12-01", DayWeather: "晴", NightWeather: "多云"},
		{Date: "2026-12-02", DayWeather: "小雨", NightWeather: "阴"},
	}

	t.Run("repeated calls reuse one execution and record weather", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchWeatherTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return weatherToolOutput(t, forecast...), nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{
				toolCall("call-1", researchWeatherTool, `{"city":"北京","start_date":"2026-12-01","days":2}`),
				toolCall("call-2", researchWeatherTool, `{"city":"北京","start_date":"2026-12-01","days":2}`),
			}},
			{Content: briefJSON(t, `{"status":"ok"}`)},
		}}
		events := &eventRecorder{}
		nodes := researchTestNodes(nil, events,
			WithToolLLM(llm), WithResearchTools(tool), WithResearchLimits(5*time.Second, 6))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if tool.callCount() != 1 {
			t.Fatalf("tool calls = %d", tool.callCount())
		}
		if out.Brief.ToolCalls != 1 || len(out.Brief.Weather) != 2 {
			t.Fatalf("brief = %+v", out.Brief)
		}
		if out.Brief.Status != domain.ResearchOK {
			t.Fatalf("status = %s", out.Brief.Status)
		}
		status, days := completedWeather(t, events)
		if status != "ok" || days != 2 {
			t.Fatalf("weather event = %s/%d", status, days)
		}
	})

	t.Run("weather tool is exposed to the research prompt", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchWeatherTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return weatherToolOutput(t, forecast...), nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{{Content: briefJSON(t, `{"status":"ok"}`)}}}
		nodes := researchTestNodes(nil, &eventRecorder{},
			WithToolLLM(llm), WithResearchTools(tool), WithResearchLimits(5*time.Second, 6))

		if _, err := nodes.Research(context.Background(), researchTestInput()); err != nil {
			t.Fatalf("research: %v", err)
		}
		req := llm.requestAt(t, 0)
		if len(req.Tools) != 1 || req.Tools[0].Name != researchWeatherTool {
			t.Fatalf("tools = %+v", req.Tools)
		}
	})

	t.Run("tool weather replaces model weather", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchWeatherTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return weatherToolOutput(t, forecast...), nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("call-1", researchWeatherTool, `{"city":"北京","start_date":"2026-12-01","days":2}`)}},
			{Content: briefJSON(t, `{"status":"ok","weather":[{"date":"1999-01-01","day_weather":"晴"}]}`)},
		}}
		nodes := researchTestNodes(nil, &eventRecorder{},
			WithToolLLM(llm), WithResearchTools(tool), WithResearchLimits(5*time.Second, 6))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if len(out.Brief.Weather) != 2 || out.Brief.Weather[0].Date != "2026-12-01" {
			t.Fatalf("weather = %+v", out.Brief.Weather)
		}
	})

	t.Run("empty tool result drops model weather and keeps the warning", func(t *testing.T) {
		warning := "天气不可用（所选日期暂无预报，AMap 当前仅提供 2026-11-20 至 2026-11-23 的预报）"
		tool := &fakeResearchTool{name: researchWeatherTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			payload, err := json.Marshal(map[string]any{
				"provider":   "amap",
				"city":       "北京",
				"days":       []domain.WeatherDay{},
				"fetched_at": "2026-11-19T00:00:00Z",
				"warnings":   []string{warning},
			})
			if err != nil {
				t.Fatalf("encode weather: %v", err)
			}
			return payload, nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("call-1", researchWeatherTool, `{"city":"北京","start_date":"2026-12-01","days":2}`)}},
			{Content: briefJSON(t, `{"status":"ok","weather":[{"date":"1999-01-01","day_weather":"晴"}]}`)},
		}}
		events := &eventRecorder{}
		nodes := researchTestNodes(nil, events,
			WithToolLLM(llm), WithResearchTools(tool), WithResearchLimits(5*time.Second, 6))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if len(out.Brief.Weather) != 0 {
			t.Fatalf("weather = %+v", out.Brief.Weather)
		}
		if !containsString(out.Brief.Warnings, warning) {
			t.Fatalf("warnings = %+v", out.Brief.Warnings)
		}
		if status, days := completedWeather(t, events); status != "empty" || days != 0 {
			t.Fatalf("weather event = %s/%d", status, days)
		}
	})

	t.Run("upstream failure only warns", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchWeatherTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"error":{"code":"upstream_unavailable","message":"weather down"}}`), nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("call-1", researchWeatherTool, `{"city":"北京","start_date":"2026-12-01","days":2}`)}},
			{Content: briefJSON(t, `{"status":"ok"}`)},
		}}
		events := &eventRecorder{}
		nodes := researchTestNodes(nil, events,
			WithToolLLM(llm), WithResearchTools(tool), WithResearchLimits(5*time.Second, 6))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if out.Brief.Status != domain.ResearchOK || len(out.Brief.Weather) != 0 {
			t.Fatalf("brief = %+v", out.Brief)
		}
		if !containsString(out.Brief.Warnings, "天气不可用（上游服务异常）") {
			t.Fatalf("warnings = %+v", out.Brief.Warnings)
		}
		if status, _ := completedWeather(t, events); status != "unavailable" {
			t.Fatalf("weather status = %s", status)
		}
	})

	t.Run("timeout envelope marks the weather status", func(t *testing.T) {
		tool := &fakeResearchTool{name: researchWeatherTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"error":{"code":"timeout","message":"weather timed out"}}`), nil
		}}
		llm := &fakeToolLLM{responses: []ToolCompletion{
			{ToolCalls: []ToolCall{toolCall("call-1", researchWeatherTool, `{"city":"北京","start_date":"2026-12-01","days":2}`)}},
			{Content: briefJSON(t, `{"status":"ok"}`)},
		}}
		events := &eventRecorder{}
		nodes := researchTestNodes(nil, events,
			WithToolLLM(llm), WithResearchTools(tool), WithResearchLimits(5*time.Second, 6))

		out, err := nodes.Research(context.Background(), researchTestInput())
		if err != nil {
			t.Fatalf("research: %v", err)
		}
		if !containsString(out.Brief.Warnings, "天气不可用（查询超时）") {
			t.Fatalf("warnings = %+v", out.Brief.Warnings)
		}
		if status, _ := completedWeather(t, events); status != "timeout" {
			t.Fatalf("weather status = %s", status)
		}
	})
}
