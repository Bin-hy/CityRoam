package tools

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"trippilot/internal/domain"
)

func TestRoute(t *testing.T) {
	from := domain.POI{ID: "a", Lat: 39.9, Lng: 116.4}
	to := domain.POI{ID: "b", Lat: 39.9, Lng: 116.41}

	t.Run("walking route", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v3/direction/walking" {
				t.Errorf("path = %s", r.URL.Path)
			}
			io.WriteString(w, `{"status":"1","route":{"paths":[{"duration":"600","distance":"800"}]}}`)
		}))
		defer server.Close()

		service := NewRouteService(NewAMapClient("test-key", WithAMapBaseURL(server.URL)), "beijing")
		transport, err := service.Route(context.Background(), from, to, domain.TransportWalk)
		if err != nil {
			t.Fatalf("route: %v", err)
		}
		if transport.DurationMin != 10 || transport.DistanceM != 800 || transport.Estimated {
			t.Fatalf("transport = %+v", transport)
		}
		if transport.Mode != domain.TransportWalk || transport.CostYuan != 0 {
			t.Fatalf("transport = %+v", transport)
		}
	})

	t.Run("transit route", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v3/direction/transit/integrated" {
				t.Errorf("path = %s", r.URL.Path)
			}
			query := r.URL.Query()
			if query.Get("city") != "beijing" || query.Get("cityd") != "beijing" {
				t.Errorf("city params = %v", query)
			}
			io.WriteString(w, `{"status":"1","route":{"transits":[{"duration":"900","distance":"5200","cost":"2"}]}}`)
		}))
		defer server.Close()

		service := NewRouteService(NewAMapClient("test-key", WithAMapBaseURL(server.URL)), "beijing")
		transport, err := service.Route(context.Background(), from, to, domain.TransportTransit)
		if err != nil {
			t.Fatalf("route: %v", err)
		}
		if transport.DurationMin != 15 || transport.DistanceM != 5200 || transport.CostYuan != 2 {
			t.Fatalf("transport = %+v", transport)
		}
	})

	t.Run("provider failure falls back", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"status":"0","infocode":"10003","info":"QUOTA"}`)
		}))
		defer server.Close()

		service := NewRouteService(NewAMapClient("test-key", WithAMapBaseURL(server.URL)), "beijing")
		transport, err := service.Route(context.Background(), from, to, domain.TransportWalk)
		if err != nil {
			t.Fatalf("route: %v", err)
		}
		if !transport.Estimated || transport.DurationMin <= 0 || transport.DistanceM <= 0 {
			t.Fatalf("transport = %+v, want estimated", transport)
		}
		if transport.Mode != domain.TransportWalk || transport.CostYuan != 0 {
			t.Fatalf("transport = %+v", transport)
		}
	})

	t.Run("no provider still estimates", func(t *testing.T) {
		service := NewRouteService(nil, "beijing")
		transport, err := service.Route(context.Background(), from, to, domain.TransportTransit)
		if err != nil {
			t.Fatalf("route: %v", err)
		}
		if !transport.Estimated || transport.CostYuan != 2 {
			t.Fatalf("transport = %+v", transport)
		}
	})

	t.Run("empty route falls back", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"status":"1","route":{"paths":[]}}`)
		}))
		defer server.Close()

		service := NewRouteService(NewAMapClient("test-key", WithAMapBaseURL(server.URL)), "beijing")
		transport, err := service.Route(context.Background(), from, to, domain.TransportWalk)
		if err != nil {
			t.Fatalf("route: %v", err)
		}
		if !transport.Estimated {
			t.Fatalf("transport = %+v, want estimated", transport)
		}
	})
}

func TestWeather(t *testing.T) {
	t.Run("forecast success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"status":"1","forecasts":[{"casts":[{"date":"2026-11-20","week":"5","dayweather":"晴","nightweather":"多云","daytemp":"12","nighttemp":"2"}]}]}`)
		}))
		defer server.Close()

		service := NewWeatherService(NewAMapClient("test-key", WithAMapBaseURL(server.URL)))
		days := service.Forecast(context.Background(), "110000")
		if len(days) != 1 || days[0].DayWeather != "晴" {
			t.Fatalf("days = %+v", days)
		}
	})

	t.Run("failure is soft", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"status":"0","infocode":"10003","info":"QUOTA"}`)
		}))
		defer server.Close()

		service := NewWeatherService(NewAMapClient("test-key", WithAMapBaseURL(server.URL)))
		if days := service.Forecast(context.Background(), "110000"); len(days) != 0 {
			t.Fatalf("days = %+v, want empty", days)
		}
	})

	t.Run("no provider", func(t *testing.T) {
		service := NewWeatherService(nil)
		if days := service.Forecast(context.Background(), "110000"); len(days) != 0 {
			t.Fatalf("days = %+v, want empty", days)
		}
	})
}
