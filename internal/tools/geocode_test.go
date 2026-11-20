package tools

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHotelGeocoder(t *testing.T) {
	t.Run("falls back to geocode when place search is empty", func(t *testing.T) {
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			switch r.URL.Path {
			case "/v3/place/text":
				io.WriteString(w, `{"status":"1","pois":[]}`)
			case "/v3/geocode/geo":
				query := r.URL.Query()
				if query.Get("address") != "全季酒店" || query.Get("city") != "北京" {
					t.Errorf("query = %v", query)
				}
				io.WriteString(w, `{"status":"1","geocodes":[
					{"formatted_address":"北京市朝阳区建国路 1 号","city":"北京市","level":"门牌号","location":"116.4700,39.9100"},
					{"formatted_address":"上海市静安区南京路","city":"上海市","level":"门牌号","location":"121.4500,31.2300"},
					{"formatted_address":"北京市海淀区中关村","city":"","level":"兴趣点","location":"116.3100,39.9800"}
				]}`)
			default:
				t.Errorf("path = %s", r.URL.Path)
			}
		}))
		defer server.Close()

		geocoder := NewHotelGeocoder(NewAMapClient("test-key", WithAMapBaseURL(server.URL)))
		hotels, err := geocoder.Resolve(context.Background(), " 全季酒店 ")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if calls != 2 {
			t.Fatalf("calls = %d", calls)
		}
		if len(hotels) != 2 {
			t.Fatalf("hotels = %+v", hotels)
		}
		first := hotels[0]
		if first.ID != "amap:116.47,39.91" || first.Name != "全季酒店" {
			t.Fatalf("hotel = %+v", first)
		}
		if first.Address != "北京市朝阳区建国路 1 号" || first.Lat != 39.91 || first.Lng != 116.47 {
			t.Fatalf("hotel = %+v", first)
		}
		if first.Provider != "amap" || first.PricePerNightYuan != 0 || !first.PriceEstimated {
			t.Fatalf("hotel flags = %+v", first)
		}
	})

	t.Run("place search resolves the actual hotel first", func(t *testing.T) {
		var geocodeCalls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v3/place/text":
				io.WriteString(w, `{"status":"1","pois":[
					{"name":"北京国际饭店·谭府餐厅","type":"餐饮服务;中餐厅","address":"建国门内大街9号","location":"116.428211,39.909898"},
					{"name":"北京饭店大堂","type":"住宿服务;宾馆酒店;五星级宾馆","address":"东长安街33号","location":"116.410325,39.908843"}
				]}`)
			case "/v3/geocode/geo":
				geocodeCalls++
				io.WriteString(w, `{"status":"1","geocodes":[]}`)
			default:
				t.Errorf("path = %s", r.URL.Path)
			}
		}))
		defer server.Close()

		geocoder := NewHotelGeocoder(NewAMapClient("test-key", WithAMapBaseURL(server.URL)))
		hotels, err := geocoder.Resolve(context.Background(), "北京饭店")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if geocodeCalls != 0 {
			t.Fatalf("geocode calls = %d", geocodeCalls)
		}
		if len(hotels) != 1 {
			t.Fatalf("hotels = %+v", hotels)
		}
		hotel := hotels[0]
		if hotel.Name != "北京饭店" || hotel.Address != "东长安街33号" {
			t.Fatalf("hotel = %+v", hotel)
		}
		if hotel.Lat != 39.908843 || hotel.Lng != 116.410325 {
			t.Fatalf("hotel = %+v", hotel)
		}
	})

	t.Run("empty query skips the request", func(t *testing.T) {
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			io.WriteString(w, `{"status":"1","pois":[],"geocodes":[]}`)
		}))
		defer server.Close()

		geocoder := NewHotelGeocoder(NewAMapClient("test-key", WithAMapBaseURL(server.URL)))
		hotels, err := geocoder.Resolve(context.Background(), "   ")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(hotels) != 0 || calls != 0 {
			t.Fatalf("hotels = %+v calls = %d", hotels, calls)
		}
	})

	t.Run("city-level geocode is rejected", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v3/place/text":
				io.WriteString(w, `{"status":"1","pois":[]}`)
			case "/v3/geocode/geo":
				io.WriteString(w, `{"status":"1","geocodes":[
					{"formatted_address":"北京市","city":"北京市","level":"省","location":"116.407387,39.904179"}
				]}`)
			}
		}))
		defer server.Close()

		geocoder := NewHotelGeocoder(NewAMapClient("test-key", WithAMapBaseURL(server.URL)))
		hotels, err := geocoder.Resolve(context.Background(), "不存在的酒店")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(hotels) != 0 {
			t.Fatalf("hotels = %+v", hotels)
		}
	})
}
