package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSearchNotesInput(t *testing.T) {
	valid := SearchNotesInput{Query: "北京 胡同 美食", Count: 8, Site: "xiaohongshu.com"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid: %v", err)
	}
	withDefaults := SearchNotesInput{Query: "北京"}
	withDefaults.applyDefaults()
	if withDefaults.Count != guideDefaultCount || withDefaults.Site != "" {
		t.Fatalf("defaults = %+v", withDefaults)
	}
	if err := withDefaults.Validate(); err != nil {
		t.Fatalf("default validate: %v", err)
	}
	for _, input := range []SearchNotesInput{
		{Query: "   ", Count: 1},
		{Query: strings.Repeat("长", 129), Count: 1},
		{Query: "q", Count: 0},
		{Query: "q", Count: 11},
		{Query: "q", Count: 1, Site: "https://xiaohongshu.com"},
		{Query: "q", Count: 1, Site: "xiaohongshu.com/path"},
		{Query: "q", Count: 1, Site: "localhost"},
		{Query: "q", Count: 1, Site: "bad site.com"},
	} {
		if err := input.Validate(); err == nil {
			t.Fatalf("input must fail: %+v", input)
		}
	}
}

func TestGuideTool(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode: %v", err)
		}
		summary := strings.Repeat("摘", guideSummaryLimit+20)
		snippet := strings.Repeat("干", guideSnippetLimit+20)
		payload := map[string]any{"results": []any{
			map[string]any{
				"title": "低分", "url": "https://example.com/low", "summary": summary,
				"snippet": snippet, "site_name": "小红书", "rerank_score": 0.5,
				"date": "2026-06-01T00:00:00Z", "authority": 1,
			},
			map[string]any{
				"title": "脚本链接", "url": "javascript:alert(1)", "summary": "bad", "rerank_score": 9.9,
			},
			map[string]any{
				"title": "高分", "url": "https://example.com/high", "summary": "好攻略",
				"snippet": "片段", "site_name": "小红书", "rerank_score": 2.4,
				"date": "2026-07-01T00:00:00Z", "authority": 3,
			},
		}}
		json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	tool, err := NewGuideTool(server.URL, "guid-key", WithGuideClock(func() time.Time {
		return time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	output, err := tool.Call(context.Background(), json.RawMessage(`{"query":" 北京 胡同 美食 ","count":3}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if gotPath != "/v1/tools/search" || gotAuth != "Bearer guid-key" {
		t.Fatalf("path=%s auth=%s", gotPath, gotAuth)
	}
	if gotBody["need_content"] != false || gotBody["count"] != float64(3) {
		t.Fatalf("body = %+v", gotBody)
	}
	query, _ := gotBody["query"].(string)
	if query != "北京 胡同 美食" {
		t.Fatalf("query = %s", query)
	}

	var decoded struct {
		Results []guideResult `json:"results"`
	}
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Results) != 2 {
		t.Fatalf("results = %+v", decoded.Results)
	}
	if decoded.Results[0].Title != "高分" || decoded.Results[1].Title != "低分" {
		t.Fatalf("order = %+v", decoded.Results)
	}
	if len([]rune(decoded.Results[1].Summary)) != guideSummaryLimit {
		t.Fatalf("summary runes = %d", len([]rune(decoded.Results[1].Summary)))
	}
	if len([]rune(decoded.Results[1].Snippet)) != guideSnippetLimit {
		t.Fatalf("snippet runes = %d", len([]rune(decoded.Results[1].Snippet)))
	}
	if decoded.Results[1].Date == nil || decoded.Results[1].Date.Format(time.RFC3339) != "2026-06-01T00:00:00Z" {
		t.Fatalf("date = %+v", decoded.Results[1].Date)
	}
	if tool.ResultCount(output) != 2 {
		t.Fatalf("count = %d", tool.ResultCount(output))
	}
	if records := tool.Evidence(nil, output); len(records) != 2 || records[0].Kind != "guide" || records[0].SourceURL != "https://example.com/high" {
		t.Fatalf("evidence = %+v", records)
	}
	if strings.Contains(string(output), "/v1/tools/fetch") {
		t.Fatal("guide output must not reference fetch")
	}
}

func TestGuideToolCustomSiteAndQuery(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		io.WriteString(w, `{"results":[{"title":"t","url":"https://example.com","summary":"s"}]}`)
	}))
	defer server.Close()
	tool, _ := NewGuideTool(server.URL, "key")
	_, err := tool.Call(context.Background(), json.RawMessage(`{
		"query":"site:mafengwo.cn 北京 路线","count":2,"site":"mafengwo.cn"
	}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if gotBody["query"] != "site:mafengwo.cn 北京 路线" {
		t.Fatalf("query duplicated site: %v", gotBody["query"])
	}
}

func TestGuideToolRejectsEmptyQueryWithoutRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()
	tool, _ := NewGuideTool(server.URL, "key")
	_, err := tool.Call(context.Background(), json.RawMessage(`{"query":"   ","count":2}`))
	var toolErr *ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != CodeInvalidInput || toolErr.Retryable {
		t.Fatalf("err = %v", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestGuideToolErrors(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		code      string
		retryable bool
	}{
		{"auth", http.StatusUnauthorized, "", CodeAuthFailed, false},
		{"rate", http.StatusTooManyRequests, "", CodeRateLimited, true},
		{"server", http.StatusServiceUnavailable, "secret body", CodeUpstreamFailure, true},
		{"bad-json", http.StatusOK, "{", CodeBadResponse, false},
		{"empty", http.StatusOK, `{"results":[]}`, CodeEmptyResult, false},
		{"bad-urls", http.StatusOK, `{"results":[{"url":"javascript:1"}]}`, CodeEmptyResult, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			secret := "guide-secret"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				io.WriteString(w, test.body)
			}))
			defer server.Close()
			tool, _ := NewGuideTool(server.URL, secret)
			_, err := tool.Call(context.Background(), json.RawMessage(`{"query":"北京","count":2}`))
			var toolErr *ToolError
			if !errors.As(err, &toolErr) {
				t.Fatalf("err = %v", err)
			}
			if toolErr.Code != test.code || toolErr.Retryable != test.retryable {
				t.Fatalf("tool error = %+v", toolErr)
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "secret body") {
				t.Fatalf("secret leaked: %v", err)
			}
		})
	}
}

func TestGuideToolCacheKey(t *testing.T) {
	tool, _ := NewGuideTool("http://localhost", "key")
	first, err := tool.CacheKey(json.RawMessage(`{"query":" 北京 胡同 ","count":0}`))
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	second, _ := tool.CacheKey(json.RawMessage(`{"query":"北京 胡同","count":8}`))
	if first != second {
		t.Fatalf("keys differ:\n%s\n%s", first, second)
	}
	if !strings.HasPrefix(first, "guide:v1::") || !strings.HasSuffix(first, ":8") {
		t.Fatalf("key = %s", first)
	}
	scoped, _ := tool.CacheKey(json.RawMessage(`{"query":"北京 胡同","count":8,"site":"xiaohongshu.com"}`))
	if scoped == first || !strings.HasPrefix(scoped, "guide:v1:xiaohongshu.com:") {
		t.Fatalf("scoped key = %s", scoped)
	}
	if tool.CacheTTL() != 6*time.Hour {
		t.Fatalf("ttl = %v", tool.CacheTTL())
	}
}
