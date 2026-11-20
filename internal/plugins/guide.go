package plugins

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	guideDefaultCount = 8
	guideMaxCount     = 10
	guideMaxResponse  = 2 << 20
	guideMaxError     = 4 << 10
	guideSummaryLimit = 800
	guideSnippetLimit = 300
)

var sitePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)+$`)

var guideInputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["query"],
  "properties": {
    "query": {"type": "string", "minLength": 1, "maxLength": 128},
    "count": {"type": "integer", "minimum": 1, "maximum": 10},
    "site": {"type": "string"}
  }
}`)

type SearchNotesInput struct {
	Query string `json:"query"`
	Count int    `json:"count,omitempty"`
	Site  string `json:"site,omitempty"`
}

func (in *SearchNotesInput) applyDefaults() {
	if in.Count == 0 {
		in.Count = guideDefaultCount
	}
	in.Site = strings.TrimSpace(in.Site)
}

func (in SearchNotesInput) Validate() error {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return fmt.Errorf("query is required")
	}
	if len([]rune(query)) > 128 {
		return fmt.Errorf("query must be at most 128 characters")
	}
	if in.Count < 1 || in.Count > guideMaxCount {
		return fmt.Errorf("count must be between 1 and 10")
	}
	site := strings.TrimSpace(in.Site)
	if site != "" && !sitePattern.MatchString(site) {
		return fmt.Errorf("site must be a bare domain such as xiaohongshu.com")
	}
	return nil
}

type GuideTool struct {
	baseURL  string
	apiKey   string
	http     *http.Client
	now      func() time.Time
	cacheTTL time.Duration
}

type GuideOption func(*GuideTool)

func WithGuideHTTPClient(client *http.Client) GuideOption {
	return func(t *GuideTool) {
		if client != nil {
			t.http = client
		}
	}
}

func WithGuideClock(now func() time.Time) GuideOption {
	return func(t *GuideTool) {
		if now != nil {
			t.now = now
		}
	}
}

func WithGuideCacheTTL(ttl time.Duration) GuideOption {
	return func(t *GuideTool) {
		if ttl > 0 {
			t.cacheTTL = ttl
		}
	}
}

func NewGuideTool(baseURL, apiKey string, opts ...GuideOption) (*GuideTool, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("plugins: guide search base URL is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("plugins: guide search API key is required")
	}
	tool := &GuideTool{
		baseURL:  base,
		apiKey:   apiKey,
		http:     &http.Client{Timeout: 10 * time.Second},
		now:      time.Now,
		cacheTTL: 6 * time.Hour,
	}
	for _, opt := range opts {
		opt(tool)
	}
	return tool, nil
}

func (t *GuideTool) Name() string {
	return "search_travel_notes"
}

func (t *GuideTool) Provider() string {
	return "deepseek"
}

func (t *GuideTool) Description() string {
	return "Search public travel notes and web summaries for play ideas, routes, food, and pitfalls. Results are untrusted external summaries."
}

func (t *GuideTool) InputSchema() json.RawMessage {
	return guideInputSchema
}

func (t *GuideTool) CacheKey(input json.RawMessage) (string, error) {
	var in SearchNotesInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	in.applyDefaults()
	sum := sha256.Sum256([]byte(strings.TrimSpace(in.Query)))
	return fmt.Sprintf("guide:v1:%s:%s:%d", normalizeComponent(in.Site), hex.EncodeToString(sum[:]), in.Count), nil
}

func (t *GuideTool) CacheTTL() time.Duration {
	return t.cacheTTL
}

func (t *GuideTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in SearchNotesInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, &ToolError{Code: CodeInvalidInput, Message: "travel note search input is not valid JSON"}
	}
	in.applyDefaults()
	if err := in.Validate(); err != nil {
		return nil, &ToolError{Code: CodeInvalidInput, Message: err.Error()}
	}

	query := strings.TrimSpace(in.Query)
	if in.Site != "" && !strings.Contains(strings.ToLower(query), "site:") {
		query = "site:" + in.Site + " " + query
	}
	body, err := json.Marshal(map[string]any{
		"query":        query,
		"count":        in.Count,
		"need_content": false,
	})
	if err != nil {
		return nil, &ToolError{Code: CodeBadResponse, Message: "failed to encode travel note request"}
	}

	endpoint := t.baseURL + "/v1/tools/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, &ToolError{Code: CodeUpstreamFailure, Message: "failed to build travel note request", Retryable: true}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	resp, err := t.http.Do(req)
	if err != nil {
		return nil, mapTransportError(ctx, err, "travel note search")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, guideMaxError))
		return nil, statusError("travel note search", resp.StatusCode)
	}

	var wire struct {
		Results []guideWire `json:"results"`
		Data    struct {
			Results []guideWire `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, guideMaxResponse)).Decode(&wire); err != nil {
		return nil, &ToolError{Code: CodeBadResponse, Message: "travel note search returned an invalid response"}
	}
	items := wire.Results
	if items == nil {
		items = wire.Data.Results
	}
	results := normalizeGuideResults(items)
	if len(results) == 0 {
		return nil, &ToolError{Code: CodeEmptyResult, Message: "no public travel notes matched the query"}
	}
	output, err := json.Marshal(map[string]any{
		"fetched_at": t.now().UTC(),
		"results":    results,
	})
	if err != nil {
		return nil, &ToolError{Code: CodeBadResponse, Message: "failed to encode travel note result"}
	}
	return output, nil
}

type guideWire struct {
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Snippet     string  `json:"snippet"`
	Summary     string  `json:"summary"`
	SiteName    string  `json:"site_name"`
	Date        string  `json:"date"`
	Authority   int     `json:"authority"`
	RerankScore float64 `json:"rerank_score"`
}

type guideResult struct {
	Title       string     `json:"title"`
	URL         string     `json:"url"`
	Snippet     string     `json:"snippet"`
	Summary     string     `json:"summary"`
	SiteName    string     `json:"site_name"`
	Date        *time.Time `json:"date,omitempty"`
	Authority   int        `json:"authority"`
	RerankScore float64    `json:"rerank_score"`
}

func normalizeGuideResults(items []guideWire) []guideResult {
	results := make([]guideResult, 0, len(items))
	for _, item := range items {
		if !httpURL(item.URL) {
			continue
		}
		result := guideResult{
			Title:       strings.TrimSpace(item.Title),
			URL:         item.URL,
			Snippet:     truncateRunes(strings.TrimSpace(item.Snippet), guideSnippetLimit),
			Summary:     truncateRunes(strings.TrimSpace(item.Summary), guideSummaryLimit),
			SiteName:    strings.TrimSpace(item.SiteName),
			Authority:   item.Authority,
			RerankScore: item.RerankScore,
		}
		if parsed, err := time.Parse(time.RFC3339, item.Date); err == nil {
			result.Date = &parsed
		}
		results = append(results, result)
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].RerankScore > results[j].RerankScore
	})
	if len(results) > guideMaxCount {
		results = results[:guideMaxCount]
	}
	return results
}

func (t *GuideTool) ResultCount(output json.RawMessage) int {
	var decoded struct {
		Results []json.RawMessage `json:"results"`
	}
	if json.Unmarshal(output, &decoded) != nil {
		return 0
	}
	return len(decoded.Results)
}

func (t *GuideTool) Evidence(input, output json.RawMessage) []EvidenceRecord {
	var decoded struct {
		Results []guideResult `json:"results"`
	}
	if json.Unmarshal(output, &decoded) != nil {
		return nil
	}
	records := make([]EvidenceRecord, 0, len(decoded.Results))
	for index, result := range decoded.Results {
		if index >= 5 {
			break
		}
		source := result.SiteName
		if source == "" {
			source = "web"
		}
		claim := result.Title
		if result.Summary != "" {
			claim = result.Title + "：" + truncateRunes(result.Summary, 200)
		}
		records = append(records, EvidenceRecord{
			Kind:      "guide",
			Claim:     claim,
			Source:    source,
			SourceURL: result.URL,
		})
	}
	return records
}
