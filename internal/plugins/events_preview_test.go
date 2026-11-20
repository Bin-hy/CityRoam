package plugins

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type plainTool struct{}

func (plainTool) Name() string                 { return "plain" }
func (plainTool) Description() string          { return "plain" }
func (plainTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (plainTool) Call(context.Context, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func TestEventPreviewQuerySummaries(t *testing.T) {
	hotel, err := NewHotelTool("http://localhost:1", "")
	if err != nil {
		t.Fatalf("hotel tool: %v", err)
	}
	guide, err := NewGuideTool("http://localhost:1", "test-key")
	if err != nil {
		t.Fatalf("guide tool: %v", err)
	}
	weather := &WeatherTool{}

	tests := []struct {
		name string
		tool interface {
			QuerySummary(json.RawMessage) string
		}
		input string
		want  string
	}{
		{
			name:  "hotel",
			tool:  hotel,
			input: `{"city":"杭州","check_in":"2026-11-21","check_out":"2026-11-23","keyword":"亲子","min_star":4,"max_price":800}`,
			want:  "杭州 · 11/21–11/23 · 2 晚 · 亲子 · 4 星以上 · 每晚 ≤¥800",
		},
		{
			name:  "guide",
			tool:  guide,
			input: `{"query":"杭州亲子","count":3}`,
			want:  "杭州亲子",
		},
		{
			name:  "weather",
			tool:  weather,
			input: `{"city":"杭州","start_date":"2026-11-21","days":3}`,
			want:  "杭州 · 11/21 起 · 3 天",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tool.QuerySummary(json.RawMessage(tt.input)); got != tt.want {
				t.Fatalf("summary = %q, want %q", got, tt.want)
			}
		})
	}

	if got := hotel.QuerySummary(json.RawMessage(`not-json`)); got != "" {
		t.Fatalf("malformed summary = %q", got)
	}
	if got := queryFor(nil, json.RawMessage(`{}`)); got != "" {
		t.Fatalf("nil tool summary = %q", got)
	}
}

func TestEventPreviewResults(t *testing.T) {
	hotel, err := NewHotelTool("http://localhost:1", "")
	if err != nil {
		t.Fatalf("hotel tool: %v", err)
	}
	guide, err := NewGuideTool("http://localhost:1", "test-key")
	if err != nil {
		t.Fatalf("guide tool: %v", err)
	}
	weather := &WeatherTool{}

	t.Run("hotel preview", func(t *testing.T) {
		output := json.RawMessage(`{"hotels":[
			{"name":"西湖边酒店","star":5,"score":4.8,"price_per_night_yuan":680,"booking_url":"https://example.com/h1"},
			{"name":"南山路民宿","star":4,"score":4.6,"price_per_night_yuan":420},
			{"name":"湖滨旅舍","star":3,"price_per_night_yuan":260},
			{"name":"不应出现","star":2,"price_per_night_yuan":100}
		]}`)
		previews := hotel.ResultPreview(output)
		if len(previews) != previewLimit {
			t.Fatalf("previews = %+v", previews)
		}
		if previews[0].Title != "西湖边酒店" || previews[0].Subtitle != "5 星 · 4.8 分" || previews[0].PriceYuan != 680 {
			t.Fatalf("first = %+v", previews[0])
		}
		if strings.Contains(string(mustJSON(t, previews)), "example.com") {
			t.Fatal("hotel preview leaked booking URL")
		}
	})

	t.Run("guide preview", func(t *testing.T) {
		output := json.RawMessage(`{"results":[
			{"title":"杭州三日路线","url":"https://example.com/g1","site_name":"小红书"},
			{"title":"西湖避坑","url":"https://example.com/g2","site_name":"小红书"},
			{"title":"龙井村散步","url":"https://example.com/g3","site_name":"马蜂窝"},
			{"title":"不应出现","url":"https://example.com/g4","site_name":"网页"}
		]}`)
		previews := guide.ResultPreview(output)
		if len(previews) != previewLimit {
			t.Fatalf("previews = %+v", previews)
		}
		if previews[0].Title != "杭州三日路线" || previews[0].Subtitle != "小红书" {
			t.Fatalf("first = %+v", previews[0])
		}
		if strings.Contains(string(mustJSON(t, previews)), "example.com") {
			t.Fatal("guide preview leaked URL")
		}
	})

	t.Run("weather preview", func(t *testing.T) {
		output := json.RawMessage(`{"days":[
			{"date":"2026-11-21","day_weather":"晴","night_weather":"多云","day_temp":"18","night_temp":"25"},
			{"date":"2026-11-22","day_weather":"小雨","night_weather":"小雨","day_temp":"14","night_temp":"19"},
			{"date":"2026-11-23","day_weather":"阴","night_weather":"阴","day_temp":"12","night_temp":"17"},
			{"date":"2026-11-24","day_weather":"晴","night_weather":"晴","day_temp":"16","night_temp":"22"}
		]}`)
		previews := weather.ResultPreview(output)
		if len(previews) != previewLimit {
			t.Fatalf("previews = %+v", previews)
		}
		if previews[0].Title != "11/21" || previews[0].Subtitle != "晴 / 多云 · 18~25°C" {
			t.Fatalf("first = %+v", previews[0])
		}
		if previews[1].Subtitle != "小雨 · 14~19°C" {
			t.Fatalf("same weather = %+v", previews[1])
		}
	})

	t.Run("caps and clips", func(t *testing.T) {
		longName := strings.Repeat("酒", 100)
		output := json.RawMessage(`{"hotels":[{"name":"` + longName + `","price_per_night_yuan":300}]}`)
		previews := previewFor(hotel, output)
		if len(previews) != 1 {
			t.Fatalf("previews = %+v", previews)
		}
		if runes := []rune(previews[0].Title); len(runes) != 81 || runes[80] != '…' {
			t.Fatalf("clipped title = %q", previews[0].Title)
		}
	})

	t.Run("empty and malformed", func(t *testing.T) {
		for _, output := range []json.RawMessage{nil, json.RawMessage(`{}`), json.RawMessage(`not-json`)} {
			if previews := previewFor(hotel, output); len(previews) != 0 {
				t.Fatalf("output %s previews = %+v", output, previews)
			}
		}
	})
}

func TestEventPreviewUnsupportedTool(t *testing.T) {
	tool := plainTool{}
	if got := queryFor(tool, json.RawMessage(`{"city":"杭州"}`)); got != "" {
		t.Fatalf("query = %q", got)
	}
	if got := previewFor(tool, json.RawMessage(`{"hotels":[]}`)); len(got) != 0 {
		t.Fatalf("previews = %+v", got)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return encoded
}
