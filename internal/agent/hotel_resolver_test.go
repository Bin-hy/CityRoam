package agent

import (
	"context"
	"errors"
	"sync"
	"testing"

	"trippilot/internal/domain"
)

type fakeHotelResolver struct {
	mu      sync.Mutex
	results map[string][]domain.Hotel
	err     error
	calls   []string
}

func (r *fakeHotelResolver) Resolve(_ context.Context, query string) ([]domain.Hotel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, query)
	if r.err != nil {
		return nil, r.err
	}
	return r.results[query], nil
}

func TestResolveSpecifiedHotels(t *testing.T) {
	existing := []domain.Hotel{{
		ID: "tool:1", Name: "全季酒店(国贸店)", Lat: 39.91, Lng: 116.47, Provider: "flyai", PricePerNightYuan: 420,
	}}
	resolver := &fakeHotelResolver{results: map[string][]domain.Hotel{
		"华尔道夫": {{ID: "amap:1", Name: "华尔道夫", Lat: 39.92, Lng: 116.46, Provider: "amap", PriceEstimated: true}},
		"文华东方": {{ID: "amap:2", Name: "文华东方", Lat: 39.93, Lng: 116.45, Provider: "amap", PriceEstimated: true},
			{ID: "amap:3", Name: "华尔道夫酒店", Lat: 39.92, Lng: 116.46, Provider: "amap", PriceEstimated: true}},
		"第三家": {{ID: "amap:4", Name: "第三家", Lat: 39.94, Lng: 116.44, Provider: "amap", PriceEstimated: true}},
	}}

	hotels, calls := resolveSpecifiedHotels(context.Background(), resolver,
		[]string{"全季酒店", "华尔道夫", "文华东方", "第三家", "华尔道夫"}, existing)

	if calls != maxHotelResolveCalls {
		t.Fatalf("calls = %d, want %d", calls, maxHotelResolveCalls)
	}
	if len(resolver.calls) != maxHotelResolveCalls {
		t.Fatalf("resolver calls = %v", resolver.calls)
	}
	if resolver.calls[0] != "华尔道夫" || resolver.calls[1] != "文华东方" {
		t.Fatalf("resolver calls = %v", resolver.calls)
	}
	if len(hotels) != 3 {
		t.Fatalf("hotels = %+v", hotels)
	}
	if hotels[0].ID != "tool:1" || hotels[0].Provider != "flyai" || hotels[0].PricePerNightYuan != 420 {
		t.Fatalf("existing tool result changed: %+v", hotels[0])
	}
	if hotels[1].Name != "华尔道夫" || hotels[2].Name != "文华东方" {
		t.Fatalf("hotels = %+v", hotels)
	}
	for _, hotel := range hotels[1:] {
		if hotel.Provider != "amap" || !hotel.PriceEstimated {
			t.Fatalf("resolved hotel flags = %+v", hotel)
		}
	}
}

func TestResolveSpecifiedHotelsWithoutResolver(t *testing.T) {
	existing := []domain.Hotel{{Name: "全季酒店", Lat: 39.91, Lng: 116.47}}
	hotels, calls := resolveSpecifiedHotels(context.Background(), nil, []string{"华尔道夫"}, existing)
	if calls != 0 || len(hotels) != 1 || hotels[0].Name != "全季酒店" {
		t.Fatalf("hotels = %+v calls = %d", hotels, calls)
	}
}

func TestResolveSpecifiedHotelsSwallowsResolverErrors(t *testing.T) {
	resolver := &fakeHotelResolver{err: errors.New("quota")}
	hotels, calls := resolveSpecifiedHotels(context.Background(), resolver, []string{"华尔道夫"}, nil)
	if calls != 1 || len(hotels) != 0 {
		t.Fatalf("hotels = %+v calls = %d", hotels, calls)
	}
}
