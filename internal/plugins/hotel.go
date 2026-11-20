package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"trippilot/internal/domain"
)

const (
	hotelDefaultLimit    = 8
	hotelMaxResponseSize = 2 << 20
	hotelMaxErrorSize    = 4 << 10
	hotelMaxEvidence     = 6
)

var hotelSortValues = map[string]bool{
	"price_asc":    true,
	"price_desc":   true,
	"rate_desc":    true,
	"distance_asc": true,
}

var hotelInputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["city", "check_in", "check_out"],
  "properties": {
    "city": {"type": "string"},
    "check_in": {"type": "string", "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
    "check_out": {"type": "string", "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
    "keyword": {"type": "string", "maxLength": 60},
    "near": {"type": "string", "maxLength": 60},
    "min_star": {"type": "integer", "minimum": 1, "maximum": 5},
    "max_price": {"type": "integer", "minimum": 0},
    "sort": {"type": "string", "enum": ["price_asc", "price_desc", "rate_desc", "distance_asc"]},
    "limit": {"type": "integer", "minimum": 1, "maximum": 10}
  }
}`)

type SearchHotelsInput struct {
	City     string `json:"city"`
	CheckIn  string `json:"check_in"`
	CheckOut string `json:"check_out"`
	Keyword  string `json:"keyword,omitempty"`
	Near     string `json:"near,omitempty"`
	MinStar  int    `json:"min_star,omitempty"`
	MaxPrice int    `json:"max_price,omitempty"`
	Sort     string `json:"sort,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

func (in *SearchHotelsInput) applyDefaults() {
	if in.Limit == 0 {
		in.Limit = hotelDefaultLimit
	}
}

func (in SearchHotelsInput) Validate() error {
	if strings.TrimSpace(in.City) == "" {
		return fmt.Errorf("city is required")
	}
	checkIn, err := time.Parse("2006-01-02", in.CheckIn)
	if err != nil {
		return fmt.Errorf("check_in must use YYYY-MM-DD")
	}
	checkOut, err := time.Parse("2006-01-02", in.CheckOut)
	if err != nil {
		return fmt.Errorf("check_out must use YYYY-MM-DD")
	}
	if !checkOut.After(checkIn) {
		return fmt.Errorf("check_out must be after check_in")
	}
	if len([]rune(in.Keyword)) > 60 {
		return fmt.Errorf("keyword must be at most 60 characters")
	}
	if len([]rune(in.Near)) > 60 {
		return fmt.Errorf("near must be at most 60 characters")
	}
	if in.MinStar < 0 || in.MinStar > 5 {
		return fmt.Errorf("min_star must be between 1 and 5")
	}
	if in.MaxPrice < 0 {
		return fmt.Errorf("max_price must be non-negative")
	}
	if in.Sort != "" && !hotelSortValues[in.Sort] {
		return fmt.Errorf("sort must be one of price_asc, price_desc, rate_desc, distance_asc")
	}
	if in.Limit < 1 || in.Limit > 10 {
		return fmt.Errorf("limit must be between 1 and 10")
	}
	return nil
}

type HotelTool struct {
	baseURL  string
	token    string
	http     *http.Client
	now      func() time.Time
	cacheTTL time.Duration
}

type HotelOption func(*HotelTool)

func WithHotelHTTPClient(client *http.Client) HotelOption {
	return func(t *HotelTool) {
		if client != nil {
			t.http = client
		}
	}
}

func WithHotelClock(now func() time.Time) HotelOption {
	return func(t *HotelTool) {
		if now != nil {
			t.now = now
		}
	}
}

func WithHotelCacheTTL(ttl time.Duration) HotelOption {
	return func(t *HotelTool) {
		if ttl > 0 {
			t.cacheTTL = ttl
		}
	}
}

func NewHotelTool(baseURL, token string, opts ...HotelOption) (*HotelTool, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("plugins: hotel tool URL is required")
	}
	tool := &HotelTool{
		baseURL:  base,
		token:    token,
		http:     &http.Client{Timeout: 10 * time.Second},
		now:      time.Now,
		cacheTTL: 30 * time.Minute,
	}
	for _, opt := range opts {
		opt(tool)
	}
	return tool, nil
}

func (t *HotelTool) Name() string {
	return "search_hotels"
}

func (t *HotelTool) Provider() string {
	return "fliggy"
}

func (t *HotelTool) Description() string {
	return "Search hotel prices and locations. Use only when the user mentions lodging, names a hotel, or asks where to stay."
}

func (t *HotelTool) InputSchema() json.RawMessage {
	return hotelInputSchema
}

func (t *HotelTool) CacheKey(input json.RawMessage) (string, error) {
	var in SearchHotelsInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	in.applyDefaults()
	return fmt.Sprintf("hotel:v1:%s:%s:%s:%s:%s:%d:%d:%s:%d",
		normalizeComponent(in.City),
		in.CheckIn,
		in.CheckOut,
		normalizeComponent(in.Keyword),
		normalizeComponent(in.Near),
		in.MinStar,
		in.MaxPrice,
		normalizeComponent(in.Sort),
		in.Limit,
	), nil
}

func (t *HotelTool) CacheTTL() time.Duration {
	return t.cacheTTL
}

func (t *HotelTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in SearchHotelsInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, &ToolError{Code: CodeInvalidInput, Message: "hotel search input is not valid JSON"}
	}
	in.applyDefaults()
	if err := in.Validate(); err != nil {
		return nil, &ToolError{Code: CodeInvalidInput, Message: err.Error()}
	}

	request := map[string]any{
		"city":      mapHotelCity(in.City),
		"check_in":  in.CheckIn,
		"check_out": in.CheckOut,
		"limit":     in.Limit,
	}
	if in.Keyword != "" {
		request["keyword"] = in.Keyword
	}
	if in.Near != "" {
		request["near"] = in.Near
	}
	if in.MinStar > 0 {
		request["min_star"] = in.MinStar
	}
	if in.MaxPrice > 0 {
		request["max_price"] = in.MaxPrice
	}
	if in.Sort != "" {
		request["sort"] = in.Sort
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, &ToolError{Code: CodeBadResponse, Message: "failed to encode hotel search request"}
	}

	endpoint := t.baseURL + "/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, &ToolError{Code: CodeUpstreamFailure, Message: "failed to build hotel search request", Retryable: true}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}

	resp, err := t.http.Do(req)
	if err != nil {
		return nil, mapTransportError(ctx, err, "hotel search")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, hotelMaxErrorSize))
		return nil, statusError("hotel search", resp.StatusCode)
	}

	var wire struct {
		Provider  string      `json:"provider"`
		FetchedAt *time.Time  `json:"fetched_at"`
		Hotels    []hotelWire `json:"hotels"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, hotelMaxResponseSize)).Decode(&wire); err != nil {
		return nil, &ToolError{Code: CodeBadResponse, Message: "hotel search returned an invalid response"}
	}
	if len(wire.Hotels) == 0 {
		return nil, &ToolError{Code: CodeEmptyResult, Message: "no hotels matched the request"}
	}

	fetchedAt := t.now().UTC()
	if wire.FetchedAt != nil {
		fetchedAt = wire.FetchedAt.UTC()
	}
	provider := wire.Provider
	if provider == "" {
		provider = t.Provider()
	}
	hotels := make([]domain.Hotel, 0, len(wire.Hotels))
	for _, item := range wire.Hotels {
		hotel := item.toDomain(in.City, provider, fetchedAt)
		if hotel == nil {
			continue
		}
		hotels = append(hotels, *hotel)
	}
	if len(hotels) == 0 {
		return nil, &ToolError{Code: CodeEmptyResult, Message: "no hotels with usable locations were returned"}
	}

	output, err := json.Marshal(map[string]any{
		"provider":   provider,
		"fetched_at": fetchedAt,
		"hotels":     hotels,
	})
	if err != nil {
		return nil, &ToolError{Code: CodeBadResponse, Message: "failed to encode hotel search result"}
	}
	return output, nil
}

type hotelWire struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Address           string     `json:"address"`
	City              string     `json:"city"`
	Lat               float64    `json:"lat"`
	Lng               float64    `json:"lng"`
	Star              int        `json:"star"`
	Score             *float64   `json:"score"`
	PricePerNightYuan int        `json:"price_per_night_yuan"`
	PriceEstimated    bool       `json:"price_estimated"`
	BookingURL        string     `json:"booking_url"`
	Provider          string     `json:"provider"`
	FetchedAt         *time.Time `json:"fetched_at"`
}

func (item hotelWire) toDomain(city, provider string, fetchedAt time.Time) *domain.Hotel {
	if item.Lat == 0 || item.Lng == 0 {
		return nil
	}
	bookingURL := ""
	if httpURL(item.BookingURL) {
		bookingURL = item.BookingURL
	}
	itemProvider := item.Provider
	if itemProvider == "" {
		itemProvider = provider
	}
	itemCity := item.City
	if itemCity == "" {
		itemCity = city
	}
	price := item.PricePerNightYuan
	if price < 0 {
		price = 0
	}
	estimated := item.PriceEstimated || price == 0
	if item.FetchedAt != nil {
		fetchedAt = item.FetchedAt.UTC()
	}
	return &domain.Hotel{
		ID:                item.ID,
		Name:              strings.TrimSpace(item.Name),
		Address:           item.Address,
		City:              itemCity,
		Lat:               item.Lat,
		Lng:               item.Lng,
		Star:              item.Star,
		Score:             item.Score,
		PricePerNightYuan: price,
		PriceEstimated:    estimated,
		BookingURL:        bookingURL,
		Provider:          itemProvider,
		FetchedAt:         fetchedAt,
	}
}

func (t *HotelTool) ResultCount(output json.RawMessage) int {
	var decoded struct {
		Hotels []json.RawMessage `json:"hotels"`
	}
	if json.Unmarshal(output, &decoded) != nil {
		return 0
	}
	return len(decoded.Hotels)
}

func (t *HotelTool) Evidence(input, output json.RawMessage) []EvidenceRecord {
	var decoded struct {
		Provider string         `json:"provider"`
		Hotels   []domain.Hotel `json:"hotels"`
	}
	if json.Unmarshal(output, &decoded) != nil {
		return nil
	}
	records := make([]EvidenceRecord, 0, hotelMaxEvidence)
	for index, hotel := range decoded.Hotels {
		if index >= hotelMaxEvidence {
			break
		}
		price := "价格未知"
		if hotel.PricePerNightYuan > 0 {
			price = fmt.Sprintf("参考价 ¥%d/晚", hotel.PricePerNightYuan)
		}
		claim := fmt.Sprintf("酒店候选：%s，%s", hotel.Name, price)
		source := hotel.Provider
		if source == "" {
			source = decoded.Provider
		}
		if source == "" {
			source = "fliggy"
		}
		records = append(records, EvidenceRecord{
			Kind:      "hotel",
			Claim:     claim,
			Source:    source,
			SourceURL: hotel.BookingURL,
		})
	}
	return records
}

func mapHotelCity(city string) string {
	switch normalizeComponent(city) {
	case "beijing":
		return "北京"
	case "shanghai":
		return "上海"
	case "guangzhou":
		return "广州"
	case "shenzhen":
		return "深圳"
	case "hangzhou":
		return "杭州"
	case "chengdu":
		return "成都"
	default:
		return strings.TrimSpace(city)
	}
}

func mapTransportError(ctx context.Context, err error, operation string) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &ToolError{Code: CodeTimeout, Message: operation + " timed out", Retryable: true}
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return &ToolError{Code: CodeTimeout, Message: operation + " timed out", Retryable: true}
	}
	return &ToolError{Code: CodeUpstreamFailure, Message: operation + " is unavailable", Retryable: true}
}

func statusError(operation string, status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return &ToolError{Code: CodeAuthFailed, Message: operation + " credentials were rejected"}
	case status == http.StatusTooManyRequests:
		return &ToolError{Code: CodeRateLimited, Message: operation + " is rate limited", Retryable: true}
	case status >= 500:
		return &ToolError{Code: CodeUpstreamFailure, Message: fmt.Sprintf("%s returned status %d", operation, status), Retryable: true}
	default:
		return &ToolError{Code: CodeInvalidInput, Message: fmt.Sprintf("%s rejected the request with status %d", operation, status)}
	}
}
