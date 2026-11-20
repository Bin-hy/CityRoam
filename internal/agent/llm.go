package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	defaultLLMTimeout     = 60 * time.Second
	defaultMaxSchemaBytes = 64 << 10
	maxResponseBytes      = 4 << 20
	maxErrorBytes         = 4 << 10
	jsonRetries           = 1
)

// Usage records what one GenerateJSON call consumed, including retries.
type Usage struct {
	Model            string        `json:"model"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	TotalTokens      int           `json:"total_tokens"`
	Calls            int           `json:"calls"`
	Attempts         int           `json:"attempts"`
	Duration         time.Duration `json:"duration"`
}

// LLMClient is the single seam through which the agent talks to an LLM.
type LLMClient interface {
	GenerateJSON(ctx context.Context, prompt string, schema json.RawMessage) (json.RawMessage, Usage, error)
}

type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolCompletionRequest struct {
	System   string
	Messages []ChatMessage
	Tools    []ToolDefinition
}

type ToolCompletion struct {
	Content   string
	ToolCalls []ToolCall
}

type ToolLLM interface {
	CompleteWithTools(ctx context.Context, req ToolCompletionRequest) (ToolCompletion, Usage, error)
}

type ResearchTool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

// OpenAIClient targets OpenAI-compatible Chat Completions endpoints such as
// DeepSeek. Only JSON output is requested; schema enforcement stays in code.
type OpenAIClient struct {
	endpoint    string
	apiKey      string
	model       string
	temperature float64
	maxTokens   int
	http        *http.Client
	logger      *slog.Logger
}

type Option func(*OpenAIClient)

func WithHTTPClient(client *http.Client) Option {
	return func(c *OpenAIClient) { c.http = client }
}

func WithLLMLogger(logger *slog.Logger) Option {
	return func(c *OpenAIClient) { c.logger = logger }
}

func WithTemperature(temperature float64) Option {
	return func(c *OpenAIClient) { c.temperature = temperature }
}

func WithMaxTokens(tokens int) Option {
	return func(c *OpenAIClient) { c.maxTokens = tokens }
}

func NewOpenAIClient(baseURL, apiKey, model string, opts ...Option) (*OpenAIClient, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("llm: base URL is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("llm: model is required")
	}
	endpoint := base
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}

	client := &OpenAIClient{
		endpoint:    endpoint,
		apiKey:      apiKey,
		model:       model,
		temperature: 0.2,
		http:        &http.Client{Timeout: defaultLLMTimeout},
		logger:      slog.Default(),
	}
	for _, opt := range opts {
		opt(client)
	}
	if client.http == nil {
		client.http = &http.Client{Timeout: defaultLLMTimeout}
	}
	return client, nil
}

// GenerateJSON asks for a JSON object. If the reply is not valid JSON, it
// retries once with the parse error appended to the prompt. Schema text is
// embedded in the prompt because OpenAI-compatible providers such as DeepSeek
// do not implement response_format json_schema.
func (c *OpenAIClient) GenerateJSON(ctx context.Context, prompt string, schema json.RawMessage) (json.RawMessage, Usage, error) {
	start := time.Now()
	usage := Usage{Model: c.model, Calls: 1}

	if len(schema) > 0 {
		if len(schema) > defaultMaxSchemaBytes {
			return nil, usage, fmt.Errorf("llm: schema exceeds %d bytes", defaultMaxSchemaBytes)
		}
		if !json.Valid(schema) {
			return nil, usage, fmt.Errorf("llm: schema is not valid JSON")
		}
	}

	requestPrompt := promptWithSchema(prompt, schema)
	var lastErr error
	for attempt := 0; attempt <= jsonRetries; attempt++ {
		usage.Attempts = attempt + 1
		content, remote, err := c.complete(ctx, requestPrompt)
		usage.PromptTokens += remote.PromptTokens
		usage.CompletionTokens += remote.CompletionTokens
		usage.TotalTokens += remote.TotalTokens
		if remote.Model != "" {
			usage.Model = remote.Model
		}
		if err != nil {
			usage.Duration = time.Since(start)
			return nil, usage, err
		}

		trimmed := stripCodeFence(content)
		if json.Valid([]byte(trimmed)) {
			usage.Duration = time.Since(start)
			return json.RawMessage(trimmed), usage, nil
		}

		lastErr = fmt.Errorf("reply is not valid JSON")
		c.logger.Warn("llm response was not valid JSON",
			"model", c.model, "attempt", usage.Attempts, "preview", preview(trimmed))
		requestPrompt = promptWithSchema(prompt, schema) +
			"\n\n上一次输出无法解析为 JSON（" + lastErr.Error() + "）。请只输出一个合法的 JSON 值，不要包含解释或 Markdown 代码块。"
	}

	usage.Duration = time.Since(start)
	return nil, usage, fmt.Errorf("llm: %v after %d attempts", lastErr, usage.Attempts)
}

type remoteUsage struct {
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}

type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

func (c *OpenAIClient) CompleteWithTools(ctx context.Context, req ToolCompletionRequest) (out ToolCompletion, usage Usage, err error) {
	start := time.Now()
	usage = Usage{Model: c.model, Calls: 1, Attempts: 1}
	defer func() { usage.Duration = time.Since(start) }()

	messages := make([]wireMessage, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		messages = append(messages, wireMessage{Role: "system", Content: req.System})
	}
	for _, message := range req.Messages {
		wire := wireMessage{Role: message.Role, Content: message.Content, ToolCallID: message.ToolCallID}
		for _, call := range message.ToolCalls {
			if call.ID == "" || call.Name == "" {
				return ToolCompletion{}, usage, fmt.Errorf("llm: tool call is missing id or name")
			}
			arguments := call.Arguments
			if len(arguments) == 0 {
				arguments = json.RawMessage(`{}`)
			}
			if !json.Valid(arguments) {
				return ToolCompletion{}, usage, fmt.Errorf("llm: tool call arguments are not valid JSON")
			}
			wire.ToolCalls = append(wire.ToolCalls, wireToolCall{
				ID:   call.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: call.Name, Arguments: string(arguments)},
			})
		}
		messages = append(messages, wire)
	}

	tools := make([]map[string]any, 0, len(req.Tools))
	for _, tool := range req.Tools {
		if tool.Name == "" {
			return ToolCompletion{}, usage, fmt.Errorf("llm: tool definition is missing name")
		}
		parameters := tool.Parameters
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{"type":"object"}`)
		}
		if !json.Valid(parameters) {
			return ToolCompletion{}, usage, fmt.Errorf("llm: tool %s parameters are not valid JSON", tool.Name)
		}
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  json.RawMessage(parameters),
			},
		})
	}

	body := map[string]any{
		"model":       c.model,
		"messages":    messages,
		"tool_choice": "auto",
		"temperature": c.temperature,
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	if c.maxTokens > 0 {
		body["max_tokens"] = c.maxTokens
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return ToolCompletion{}, usage, fmt.Errorf("llm: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return ToolCompletion{}, usage, fmt.Errorf("llm: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return ToolCompletion{}, usage, fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBytes))
		return ToolCompletion{}, usage, fmt.Errorf("llm: status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}

	var decoded struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content   *string        `json:"content"`
				ToolCalls []wireToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage remoteUsage `json:"usage"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&decoded); err != nil {
		return ToolCompletion{}, usage, fmt.Errorf("llm: decode response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return ToolCompletion{}, usage, fmt.Errorf("llm: response has no choices")
	}
	usage.PromptTokens = decoded.Usage.PromptTokens
	usage.CompletionTokens = decoded.Usage.CompletionTokens
	usage.TotalTokens = decoded.Usage.TotalTokens
	usage.Model = decoded.Model
	if usage.Model == "" {
		usage.Model = c.model
	}

	message := decoded.Choices[0].Message
	out = ToolCompletion{}
	if message.Content != nil {
		out.Content = *message.Content
	}
	for _, call := range message.ToolCalls {
		if call.Function.Name == "" {
			return ToolCompletion{}, usage, fmt.Errorf("llm: response tool call is missing name")
		}
		arguments := strings.TrimSpace(call.Function.Arguments)
		if !json.Valid([]byte(arguments)) {
			return ToolCompletion{}, usage, fmt.Errorf("llm: tool %s returned invalid JSON arguments", call.Function.Name)
		}
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: json.RawMessage(arguments),
		})
	}
	if strings.TrimSpace(out.Content) == "" && len(out.ToolCalls) == 0 {
		return ToolCompletion{}, usage, fmt.Errorf("llm: response has neither content nor tool calls")
	}

	c.logger.Info("llm tool request",
		"model", usage.Model,
		"prompt_tokens", usage.PromptTokens,
		"completion_tokens", usage.CompletionTokens,
		"tool_calls", len(out.ToolCalls),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return out, usage, nil
}

func (c *OpenAIClient) complete(ctx context.Context, prompt string) (string, remoteUsage, error) {
	var usage remoteUsage

	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     c.temperature,
	}
	if c.maxTokens > 0 {
		body["max_tokens"] = c.maxTokens
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", usage, fmt.Errorf("llm: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return "", usage, fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return "", usage, fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBytes))
		return "", usage, fmt.Errorf("llm: status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}

	var decoded struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&decoded); err != nil {
		return "", usage, fmt.Errorf("llm: decode response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", usage, fmt.Errorf("llm: response has no choices")
	}

	usage = remoteUsage{
		Model:            decoded.Model,
		PromptTokens:     decoded.Usage.PromptTokens,
		CompletionTokens: decoded.Usage.CompletionTokens,
		TotalTokens:      decoded.Usage.TotalTokens,
	}
	if usage.Model == "" {
		usage.Model = c.model
	}
	content := decoded.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		return "", usage, fmt.Errorf("llm: response content is empty")
	}

	c.logger.Info("llm request",
		"model", usage.Model,
		"prompt_tokens", usage.PromptTokens,
		"completion_tokens", usage.CompletionTokens,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return content, usage, nil
}

func promptWithSchema(prompt string, schema json.RawMessage) string {
	if len(schema) == 0 {
		return prompt
	}
	return prompt + "\n\n输出必须符合以下 JSON Schema：\n" + string(schema)
}

func stripCodeFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	if newline := strings.IndexByte(trimmed, '\n'); newline >= 0 {
		trimmed = trimmed[newline+1:]
	}
	trimmed = strings.TrimSuffix(strings.TrimSpace(trimmed), "```")
	return strings.TrimSpace(trimmed)
}

func preview(content string) string {
	const limit = 200
	if len(content) <= limit {
		return content
	}
	return content[:limit] + "..."
}
