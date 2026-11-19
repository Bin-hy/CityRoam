package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAMapBaseURL = "https://restapi.amap.com"
	defaultAMapTimeout = 5 * time.Second
	maxAMapBodyBytes   = 1 << 20
)

var ErrQuota = errors.New("amap: quota exceeded")

type APIError struct {
	InfoCode string
	Info     string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("amap: %s (%s)", e.Info, e.InfoCode)
}

type AMapClient struct {
	key     string
	baseURL string
	client  *http.Client
}

type AMapOption func(*AMapClient)

func WithAMapBaseURL(baseURL string) AMapOption {
	return func(c *AMapClient) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

func WithAMapHTTPClient(client *http.Client) AMapOption {
	return func(c *AMapClient) {
		c.client = client
	}
}

func NewAMapClient(key string, opts ...AMapOption) *AMapClient {
	client := &AMapClient{
		key:     key,
		baseURL: defaultAMapBaseURL,
		client:  &http.Client{Timeout: defaultAMapTimeout},
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

type AMapPOI struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Address  string  `json:"address"`
	Tel      string  `json:"tel"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
}

type Geocode struct {
	FormattedAddress string  `json:"formatted_address"`
	Province         string  `json:"province"`
	City             string  `json:"city"`
	District         string  `json:"district"`
	Lat              float64 `json:"lat"`
	Lng              float64 `json:"lng"`
}

type WeatherDay struct {
	Date         string `json:"date"`
	Week         string `json:"week"`
	DayWeather   string `json:"day_weather"`
	NightWeather string `json:"night_weather"`
	DayTemp      string `json:"day_temp"`
	NightTemp    string `json:"night_temp"`
}

func (c *AMapClient) SearchPOI(ctx context.Context, keywords, city string) ([]AMapPOI, error) {
	params := url.Values{}
	params.Set("keywords", keywords)
	params.Set("city", city)
	params.Set("offset", "25")
	params.Set("page", "1")
	params.Set("extensions", "base")

	var payload struct {
		POIs []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Type     string `json:"type"`
			Address  string `json:"address"`
			Location string `json:"location"`
			Tel      string `json:"tel"`
		} `json:"pois"`
	}
	if err := c.get(ctx, "/v3/place/text", params, &payload); err != nil {
		return nil, err
	}

	pois := make([]AMapPOI, 0, len(payload.POIs))
	for _, item := range payload.POIs {
		lat, lng, err := parseLocation(item.Location)
		if err != nil {
			continue
		}
		pois = append(pois, AMapPOI{
			ID:       item.ID,
			Name:     item.Name,
			Category: item.Type,
			Address:  item.Address,
			Tel:      item.Tel,
			Lat:      lat,
			Lng:      lng,
		})
	}
	return pois, nil
}

func (c *AMapClient) Geocode(ctx context.Context, address, city string) ([]Geocode, error) {
	params := url.Values{}
	params.Set("address", address)
	if city != "" {
		params.Set("city", city)
	}

	var payload struct {
		Geocodes []struct {
			FormattedAddress string `json:"formatted_address"`
			Province         string `json:"province"`
			City             string `json:"city"`
			District         string `json:"district"`
			Location         string `json:"location"`
		} `json:"geocodes"`
	}
	if err := c.get(ctx, "/v3/geocode/geo", params, &payload); err != nil {
		return nil, err
	}

	results := make([]Geocode, 0, len(payload.Geocodes))
	for _, item := range payload.Geocodes {
		lat, lng, err := parseLocation(item.Location)
		if err != nil {
			continue
		}
		results = append(results, Geocode{
			FormattedAddress: item.FormattedAddress,
			Province:         item.Province,
			City:             item.City,
			District:         item.District,
			Lat:              lat,
			Lng:              lng,
		})
	}
	return results, nil
}

func (c *AMapClient) Weather(ctx context.Context, city string) ([]WeatherDay, error) {
	params := url.Values{}
	params.Set("city", city)
	params.Set("extensions", "all")

	var payload struct {
		Forecasts []struct {
			Casts []struct {
				Date         string `json:"date"`
				Week         string `json:"week"`
				DayWeather   string `json:"dayweather"`
				NightWeather string `json:"nightweather"`
				DayTemp      string `json:"daytemp"`
				NightTemp    string `json:"nighttemp"`
			} `json:"casts"`
		} `json:"forecasts"`
	}
	if err := c.get(ctx, "/v3/weather/weatherInfo", params, &payload); err != nil {
		return nil, err
	}

	days := make([]WeatherDay, 0)
	for _, forecast := range payload.Forecasts {
		for _, cast := range forecast.Casts {
			days = append(days, WeatherDay{
				Date:         cast.Date,
				Week:         cast.Week,
				DayWeather:   cast.DayWeather,
				NightWeather: cast.NightWeather,
				DayTemp:      cast.DayTemp,
				NightTemp:    cast.NightTemp,
			})
		}
	}
	return days, nil
}

func (c *AMapClient) get(ctx context.Context, path string, params url.Values, out any) error {
	if c.key == "" {
		return errors.New("amap: key is required")
	}
	params.Set("key", c.key)
	params.Set("output", "JSON")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("amap: build request: %w", err)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("amap: request %s: %w", path, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxAMapBodyBytes))
	if err != nil {
		return fmt.Errorf("amap: read response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("amap: unexpected status %d", response.StatusCode)
	}

	var status struct {
		Status   string `json:"status"`
		InfoCode string `json:"infocode"`
		Info     string `json:"info"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return fmt.Errorf("amap: decode status: %w", err)
	}
	if status.Status != "1" {
		if status.InfoCode == "10003" || status.InfoCode == "10004" {
			return fmt.Errorf("%w: %s", ErrQuota, status.Info)
		}
		return &APIError{InfoCode: status.InfoCode, Info: status.Info}
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("amap: decode response: %w", err)
	}
	return nil
}

func parseLocation(location string) (lat, lng float64, err error) {
	parts := strings.Split(location, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("amap: invalid location %q", location)
	}
	lng, err = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("amap: parse longitude: %w", err)
	}
	lat, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("amap: parse latitude: %w", err)
	}
	return lat, lng, nil
}
