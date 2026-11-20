package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLLMGenerateJSON(t *testing.T) {
	var gotPath, gotAuth, gotModel, gotFormat, gotPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Model          string `json:"model"`
			ResponseFormat struct {
				Type string `json:"type"`
			} `json:"response_format"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		gotModel = body.Model
		gotFormat = body.ResponseFormat.Type
		if len(body.Messages) > 0 {
			gotPrompt = body.Messages[0].Content
		}
		io.WriteString(w, `{
			"model": "deepseek-chat",
			"choices": [{"message": {"content": "{\"city\":\"北京\",\"days\":2}"}}],
			"usage": {"prompt_tokens": 120, "completion_tokens": 30, "total_tokens": 150}
		}`)
	}))
	defer server.Close()

	client, err := NewOpenAIClient(server.URL, "secret-key", "deepseek-chat", WithLLMLogger(discardLogger()))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	schema := json.RawMessage(`{"type":"object","required":["city","days"]}`)
	payload, usage, err := client.GenerateJSON(context.Background(), "解析约束", schema)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if string(payload) != `{"city":"北京","days":2}` {
		t.Fatalf("payload = %s", payload)
	}
	if usage.Model != "deepseek-chat" || usage.PromptTokens != 120 || usage.CompletionTokens != 30 || usage.TotalTokens != 150 {
		t.Fatalf("usage = %+v", usage)
	}
	if usage.Attempts != 1 || usage.Duration <= 0 {
		t.Fatalf("usage = %+v", usage)
	}
	if gotPath != "/chat/completions" || gotAuth != "Bearer secret-key" || gotModel != "deepseek-chat" {
		t.Fatalf("path=%s auth=%s model=%s", gotPath, gotAuth, gotModel)
	}
	if gotFormat != "json_object" {
		t.Fatalf("response_format = %s", gotFormat)
	}
	if !strings.Contains(gotPrompt, `"required":["city","days"]`) {
		t.Fatalf("schema missing from prompt: %s", gotPrompt)
	}
}

func TestLLMRetriesInvalidJSON(t *testing.T) {
	var calls atomic.Int32
	var secondPrompt atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		if calls.Add(1) == 1 {
			io.WriteString(w, `{"choices":[{"message":{"content":"抱歉，我不能输出 JSON"}}],"usage":{"total_tokens":10}}`)
			return
		}
		secondPrompt.Store(body.Messages[0].Content)
		fenced, _ := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "```json\n{\"ok\":true}\n```"}}},
		})
		w.Write(fenced)
	}))
	defer server.Close()

	client, err := NewOpenAIClient(server.URL, "", "deepseek-chat", WithLLMLogger(discardLogger()))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	payload, usage, err := client.GenerateJSON(context.Background(), "解析", nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if string(payload) != `{"ok":true}` {
		t.Fatalf("payload = %s", payload)
	}
	if calls.Load() != 2 || usage.Attempts != 2 {
		t.Fatalf("calls = %d, attempts = %d", calls.Load(), usage.Attempts)
	}
	if usage.TotalTokens != 10 {
		t.Fatalf("usage = %+v", usage)
	}
	prompt, _ := secondPrompt.Load().(string)
	if !strings.Contains(prompt, "无法解析为 JSON") {
		t.Fatalf("retry prompt missing error: %s", prompt)
	}
}

func TestLLMInvalidJSONExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"still not json"}}]}`)
	}))
	defer server.Close()

	client, err := NewOpenAIClient(server.URL, "", "m", WithLLMLogger(discardLogger()))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	_, usage, err := client.GenerateJSON(context.Background(), "解析", nil)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("err = %v", err)
	}
	if usage.Attempts != 2 {
		t.Fatalf("attempts = %d", usage.Attempts)
	}
}

func TestLLMTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer server.Close()

	client, err := NewOpenAIClient(server.URL, "", "m",
		WithLLMLogger(discardLogger()),
		WithHTTPClient(&http.Client{Timeout: 30 * time.Millisecond}),
	)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	start := time.Now()
	_, _, err = client.GenerateJSON(context.Background(), "解析", nil)
	if err == nil {
		t.Fatal("want timeout error")
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("elapsed = %v", elapsed)
	}
}

func TestLLMHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, "upstream unavailable")
	}))
	defer server.Close()

	client, err := NewOpenAIClient(server.URL, "", "m", WithLLMLogger(discardLogger()))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	_, _, err = client.GenerateJSON(context.Background(), "解析", nil)
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v", err)
	}
}

func TestLLMRequiresConfig(t *testing.T) {
	if _, err := NewOpenAIClient("", "", "m"); err == nil {
		t.Fatal("want error for empty base URL")
	}
	if _, err := NewOpenAIClient("http://localhost", "", ""); err == nil {
		t.Fatal("want error for empty model")
	}
	if _, err := NewOpenAIClient("http://localhost/v1", "", "m"); err != nil {
		t.Fatalf("client: %v", err)
	}
	client, _ := NewOpenAIClient("http://localhost/v1/", "", "m")
	if client.endpoint != "http://localhost/v1/chat/completions" {
		t.Fatalf("endpoint = %s", client.endpoint)
	}
}

func TestCompleteWithTools(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		io.WriteString(w, `{
			"model": "deepseek-chat",
			"choices": [{"message": {
				"content": "",
				"tool_calls": [
					{"id":"c1","type":"function","function":{"name":"search_hotels","arguments":"{\"city\":\"北京\"}"}},
					{"id":"c2","type":"function","function":{"name":"search_travel_notes","arguments":"{\"query\":\"胡同\"}"}}
				]
			}}],
			"usage": {"prompt_tokens": 80, "completion_tokens": 12, "total_tokens": 92}
		}`)
	}))
	defer server.Close()

	client, err := NewOpenAIClient(server.URL, "secret-key", "deepseek-chat", WithLLMLogger(discardLogger()))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	result, usage, err := client.CompleteWithTools(context.Background(), ToolCompletionRequest{
		System: "system",
		Messages: []ChatMessage{
			{Role: "user", Content: "北京两天"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "search_hotels", Arguments: json.RawMessage(`{"city":"beijing"}`)}}},
			{Role: "tool", ToolCallID: "c1", Content: `{"hotels":[]}`},
		},
		Tools: []ToolDefinition{{
			Name:        "search_hotels",
			Description: "search",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("tool calls = %+v", result.ToolCalls)
	}
	if result.ToolCalls[0].Name != "search_hotels" || string(result.ToolCalls[0].Arguments) != `{"city":"北京"}` {
		t.Fatalf("first call = %+v", result.ToolCalls[0])
	}
	if result.ToolCalls[1].Name != "search_travel_notes" {
		t.Fatalf("second call = %+v", result.ToolCalls[1])
	}
	if usage.TotalTokens != 92 || usage.Calls != 1 || usage.Attempts != 1 {
		t.Fatalf("usage = %+v", usage)
	}
	if _, found := gotBody["response_format"]; found {
		t.Fatal("tool completion must not send response_format")
	}
	if gotBody["tool_choice"] != "auto" {
		t.Fatalf("tool_choice = %v", gotBody["tool_choice"])
	}
	if _, ok := gotBody["tools"].([]any); !ok {
		t.Fatalf("tools = %#v", gotBody["tools"])
	}
	messages, ok := gotBody["messages"].([]any)
	if !ok || len(messages) != 4 {
		t.Fatalf("messages = %#v", gotBody["messages"])
	}
	system, _ := messages[0].(map[string]any)
	if system["role"] != "system" || system["content"] != "system" {
		t.Fatalf("system message = %#v", messages[0])
	}
	assistant, _ := messages[2].(map[string]any)
	calls, _ := assistant["tool_calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("assistant tool_calls = %#v", assistant["tool_calls"])
	}
	toolMessage, _ := messages[3].(map[string]any)
	if toolMessage["tool_call_id"] != "c1" || toolMessage["role"] != "tool" {
		t.Fatalf("tool message = %#v", messages[3])
	}
}

func TestCompleteWithToolsPlainContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"status\":\"ok\"}"}}]}`)
	}))
	defer server.Close()

	client, _ := NewOpenAIClient(server.URL, "", "m", WithLLMLogger(discardLogger()))
	result, _, err := client.CompleteWithTools(context.Background(), ToolCompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.Content != `{"status":"ok"}` || len(result.ToolCalls) != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestCompleteWithToolsRejectsInvalidArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"tool_calls":[{"id":"c1","function":{"name":"x","arguments":"not-json"}}]}}]}`)
	}))
	defer server.Close()

	client, _ := NewOpenAIClient(server.URL, "", "m", WithLLMLogger(discardLogger()))
	_, _, err := client.CompleteWithTools(context.Background(), ToolCompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid JSON arguments") {
		t.Fatalf("err = %v", err)
	}
}
