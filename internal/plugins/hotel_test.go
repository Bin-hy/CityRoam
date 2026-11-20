package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"trippilot/internal/domain"
)

func TestSearchHotelsInput(t *testing.T) {
	valid := SearchHotelsInput{
		City: "北京", CheckIn: "2026-11-21", CheckOut: "2026-11-23",
		Keyword: "王府井", Near: "故宫", MinStar: 4, MaxPrice: 900,
		Sort: "price_asc", Limit: 8,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid: %v", err)
	}
	cases := []SearchHotelsInput{
		{City: "", CheckIn: "2026-11-21", CheckOut: "2026-11-23", Limit: 1},
		{City: "北京", CheckIn: "11/21", CheckOut: "2026-11-23", Limit: 1},
		{City: "北京", CheckIn: "2026-11-23", CheckOut: "2026-11-21", Limit: 1},
		{City: "北京", CheckIn: "2026-11-21", CheckOut: "2026-11-23", MinStar: 6, Limit: 1},
		{City: "北京", CheckIn: "2026-11-21", CheckOut: "2026-11-23", MaxPrice: -1, Limit: 1},
		{City: "北京", CheckIn: "2026-11-21", CheckOut: "2026-11-23", Sort: "cheapest", Limit: 1},
		{City: "北京", CheckIn: "2026-11-21", CheckOut: "2026-11-23", Limit: 0},
		{City: "北京", CheckIn: "2026-11-21", CheckOut: "2026-11-23", Limit: 11},
		{City: "北京", CheckIn: "2026-11-21", CheckOut: "2026-11-23", Keyword: strings.Repeat("长", 61), Limit: 1},
	}
	for i, input := range cases {
		if err := input.Validate(); err == nil {
			t.Fatalf("case %d must fail: %+v", i, input)
		}
	}
	limit := valid
	limit.Limit = 0
	limit.applyDefaults()
	if err := limit.Validate(); err != nil || limit.Limit != hotelDefaultLimit {
		t.Fatalf("defaults: %+v %v", limit, err)
	}

	tool, err := NewHotelTool("http://localhost", "token")
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	if tool.Name() != "search_hotels" || tool.Provider() != "fliggy" {
		t.Fatalf("tool identity = %s %s", tool.Name(), tool.Provider())
	}
	schema := string(tool.InputSchema())
	for _, want := range []string{`"city"`, `"check_in"`, `"check_out"`, `"limit"`, `"distance_asc"`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema missing %s: %s", want, schema)
		}
	}
}

func TestHotelTool(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode: %v", err)
		}
		io.WriteString(w, `{
			"provider":"fliggy",
			"fetched_at":"2026-11-20T10:00:00Z",
			"hotels":[
				{"id":"h1","name":"一号酒店","city":"北京","lat":39.91,"lng":116.4,"star":5,"score":4.8,"price_per_night_yuan":780,"booking_url":"https://example.com/h1"},
				{"id":"h2","name":"无坐标酒店","lat":0,"lng":116.4,"price_per_night_yuan":500},
				{"id":"h3","name":"无价酒店","lat":39.92,"lng":116.41,"price_per_night_yuan":0,"booking_url":"javascript:alert(1)"}
			]
		}`)
	}))
	defer server.Close()

	tool, err := NewHotelTool(server.URL, "tok", WithHotelClock(func() time.Time {
		return time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	output, err := tool.Call(context.Background(), json.RawMessage(`{
		"city":"beijing","check_in":"2026-11-21","check_out":"2026-11-23",
		"keyword":"王府井","near":"故宫","min_star":4,"max_price":900,"sort":"price_asc","limit":8
	}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if gotPath != "/search" || gotAuth != "Bearer tok" {
		t.Fatalf("path=%s auth=%s", gotPath, gotAuth)
	}
	if gotBody["city"] != "北京" || gotBody["check_in"] != "2026-11-21" || gotBody["near"] != "故宫" {
		t.Fatalf("body = %+v", gotBody)
	}
	if gotBody["min_star"] != float64(4) || gotBody["max_price"] != float64(900) || gotBody["limit"] != float64(8) {
		t.Fatalf("body = %+v", gotBody)
	}

	var decoded struct {
		Provider  string         `json:"provider"`
		FetchedAt time.Time      `json:"fetched_at"`
		Hotels    []domain.Hotel `json:"hotels"`
	}
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(decoded.Hotels) != 2 || decoded.Provider != "fliggy" {
		t.Fatalf("decoded = %+v", decoded)
	}
	if decoded.Hotels[0].ID != "h1" || decoded.Hotels[0].BookingURL != "https://example.com/h1" {
		t.Fatalf("h1 = %+v", decoded.Hotels[0])
	}
	if decoded.Hotels[1].PricePerNightYuan != 0 || !decoded.Hotels[1].PriceEstimated {
		t.Fatalf("h3 price = %+v", decoded.Hotels[1])
	}
	if decoded.Hotels[1].BookingURL != "" {
		t.Fatalf("javascript url kept: %+v", decoded.Hotels[1])
	}
	if tool.ResultCount(output) != 2 {
		t.Fatalf("result count = %d", tool.ResultCount(output))
	}
	if records := tool.Evidence(nil, output); len(records) != 2 || records[0].Kind != "hotel" || records[0].SourceURL != "https://example.com/h1" {
		t.Fatalf("evidence = %+v", records)
	}
}

func TestHotelToolRejectsInvalidInputWithoutRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	tool, _ := NewHotelTool(server.URL, "")
	for _, input := range []string{
		`{"city":"","check_in":"2026-11-21","check_out":"2026-11-23"}`,
		`{"city":"北京","check_in":"tomorrow","check_out":"2026-11-23"}`,
		`not-json`,
	} {
		_, err := tool.Call(context.Background(), json.RawMessage(input))
		var toolErr *ToolError
		if !errors.As(err, &toolErr) || toolErr.Code != CodeInvalidInput || toolErr.Retryable {
			t.Fatalf("input %s err=%v", input, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d", requests.Load())
	}
}

func TestHotelToolErrors(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		code      string
		retryable bool
	}{
		{"auth", http.StatusUnauthorized, "", CodeAuthFailed, false},
		{"forbidden", http.StatusForbidden, "", CodeAuthFailed, false},
		{"rate", http.StatusTooManyRequests, "", CodeRateLimited, true},
		{"server", http.StatusBadGateway, "upstream secret", CodeUpstreamFailure, true},
		{"bad-json", http.StatusOK, "{", CodeBadResponse, false},
		{"empty", http.StatusOK, `{"hotels":[]}`, CodeEmptyResult, false},
		{"no-coords", http.StatusOK, `{"hotels":[{"name":"x","lat":0,"lng":0}]}`, CodeEmptyResult, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			secret := "tok-secret"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := test.body
				if test.status >= 500 {
					body += secret
				}
				w.WriteHeader(test.status)
				io.WriteString(w, body)
			}))
			defer server.Close()
			tool, _ := NewHotelTool(server.URL, secret)
			_, err := tool.Call(context.Background(), json.RawMessage(`{
				"city":"北京","check_in":"2026-11-21","check_out":"2026-11-23","limit":4
			}`))
			var toolErr *ToolError
			if !errors.As(err, &toolErr) {
				t.Fatalf("err = %v", err)
			}
			if toolErr.Code != test.code || toolErr.Retryable != test.retryable {
				t.Fatalf("tool error = %+v", toolErr)
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "upstream secret") {
				t.Fatalf("secret leaked: %v", err)
			}
		})
	}
}

func TestHotelToolCacheKey(t *testing.T) {
	tool, _ := NewHotelTool("http://localhost", "")
	first, err := tool.CacheKey(json.RawMessage(`{
		"city":" Beijing ","check_in":"2026-11-21","check_out":"2026-11-23","keyword":" Hot Spring ","limit":0
	}`))
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	second, _ := tool.CacheKey(json.RawMessage(`{
		"city":"beijing","check_in":"2026-11-21","check_out":"2026-11-23","keyword":"hot spring","limit":8
	}`))
	if first != second {
		t.Fatalf("keys differ:\n%s\n%s", first, second)
	}
	if !strings.HasPrefix(first, "hotel:v1:beijing:2026-11-21:2026-11-23:hot spring::0:0::8") {
		t.Fatalf("key = %s", first)
	}
	if tool.CacheTTL() != 30*time.Minute {
		t.Fatalf("ttl = %v", tool.CacheTTL())
	}
}
