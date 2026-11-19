package tools

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAMap(t *testing.T) {
	t.Run("search success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v3/place/text" {
				t.Errorf("path = %s", r.URL.Path)
			}
			query := r.URL.Query()
			if query.Get("key") != "test-key" || query.Get("keywords") != "故宫" || query.Get("city") != "beijing" {
				t.Errorf("query = %v", query)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"status":"1","info":"OK","infocode":"10000","pois":[
				{"id":"B00001","name":"故宫","type":"风景名胜","address":"景山前街4号","location":"116.3972,39.9163","tel":"010"}
			]}`)
		}))
		defer server.Close()

		client := NewAMapClient("test-key", WithAMapBaseURL(server.URL))
		pois, err := client.SearchPOI(context.Background(), "故宫", "beijing")
		if err != nil {
			t.Fatalf("search POI: %v", err)
		}
		if len(pois) != 1 {
			t.Fatalf("pois = %+v, want 1", pois)
		}
		if pois[0].ID != "B00001" || pois[0].Lat != 39.9163 || pois[0].Lng != 116.3972 {
			t.Fatalf("poi = %+v", pois[0])
		}
	})

	t.Run("geocode success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"status":"1","geocodes":[{"formatted_address":"北京市东城区景山前街4号","province":"北京市","city":"北京市","district":"东城区","location":"116.3972,39.9163"}]}`)
		}))
		defer server.Close()

		client := NewAMapClient("test-key", WithAMapBaseURL(server.URL))
		results, err := client.Geocode(context.Background(), "故宫", "beijing")
		if err != nil {
			t.Fatalf("geocode: %v", err)
		}
		if len(results) != 1 || results[0].Lat != 39.9163 || results[0].Lng != 116.3972 {
			t.Fatalf("results = %+v", results)
		}
	})

	t.Run("weather success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v3/weather/weatherInfo" {
				t.Errorf("path = %s", r.URL.Path)
			}
			io.WriteString(w, `{"status":"1","forecasts":[{"casts":[
				{"date":"2026-11-20","week":"5","dayweather":"晴","nightweather":"多云","daytemp":"12","nighttemp":"2"}
			]}]}`)
		}))
		defer server.Close()

		client := NewAMapClient("test-key", WithAMapBaseURL(server.URL))
		days, err := client.Weather(context.Background(), "110000")
		if err != nil {
			t.Fatalf("weather: %v", err)
		}
		if len(days) != 1 || days[0].DayWeather != "晴" || days[0].NightTemp != "2" {
			t.Fatalf("days = %+v", days)
		}
	})

	t.Run("quota error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"status":"0","infocode":"10003","info":"DAILY_QUERY_OVER_LIMIT"}`)
		}))
		defer server.Close()

		client := NewAMapClient("test-key", WithAMapBaseURL(server.URL))
		_, err := client.SearchPOI(context.Background(), "故宫", "beijing")
		if !errors.Is(err, ErrQuota) {
			t.Fatalf("err = %v, want quota error", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			io.WriteString(w, `{"status":"1","pois":[]}`)
		}))
		defer server.Close()

		client := NewAMapClient("test-key",
			WithAMapBaseURL(server.URL),
			WithAMapHTTPClient(&http.Client{Timeout: 30 * time.Millisecond}),
		)
		if _, err := client.SearchPOI(context.Background(), "故宫", "beijing"); err == nil {
			t.Fatal("expected timeout error")
		}
	})

	t.Run("missing key", func(t *testing.T) {
		client := NewAMapClient("")
		if _, err := client.SearchPOI(context.Background(), "故宫", "beijing"); err == nil {
			t.Fatal("expected missing key error")
		}
	})
}
