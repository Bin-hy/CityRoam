package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"trippilot/internal/domain"
)

const (
	geocodeHotelCity  = "北京"
	maxGeocodeHotels  = 5
	geocodeHotelPrice = 0
)

// HotelGeocoder resolves a specified hotel name to coordinates through AMap.
type HotelGeocoder struct {
	amap *AMapClient
	city string
}

func NewHotelGeocoder(amap *AMapClient) *HotelGeocoder {
	return &HotelGeocoder{amap: amap, city: geocodeHotelCity}
}

func (g *HotelGeocoder) Resolve(ctx context.Context, query string) ([]domain.Hotel, error) {
	query = strings.TrimSpace(query)
	if query == "" || g.amap == nil {
		return nil, nil
	}
	// Place search returns the actual hotel; geocoding a bare name often
	// degrades to a city-level point, which would anchor the day incorrectly.
	if pois, err := g.amap.SearchPOI(ctx, query, g.city); err == nil {
		if hotels := hotelPOIMatches(query, pois); len(hotels) > 0 {
			return hotels, nil
		}
	}

	results, err := g.amap.Geocode(ctx, query, g.city)
	if err != nil {
		return nil, err
	}
	hotels := make([]domain.Hotel, 0, len(results))
	for _, result := range results {
		if result.City != "" && !strings.Contains(result.City, geocodeHotelCity) {
			continue
		}
		if coarseGeocode(result.Level) {
			continue
		}
		hotels = append(hotels, domain.Hotel{
			ID:                fmt.Sprintf("amap:%s,%s", formatCoord(result.Lng), formatCoord(result.Lat)),
			Name:              query,
			Address:           result.FormattedAddress,
			City:              result.City,
			Lat:               result.Lat,
			Lng:               result.Lng,
			PricePerNightYuan: geocodeHotelPrice,
			PriceEstimated:    true,
			Provider:          "amap",
		})
		if len(hotels) >= maxGeocodeHotels {
			break
		}
	}
	return hotels, nil
}

func hotelPOIMatches(query string, pois []AMapPOI) []domain.Hotel {
	normalized := normalizeHotelQuery(query)
	if normalized == "" {
		return nil
	}
	hotels := make([]domain.Hotel, 0, len(pois))
	for _, poi := range pois {
		if !strings.Contains(poi.Category, "住宿") {
			continue
		}
		name := normalizeHotelQuery(poi.Name)
		if name == "" || (!strings.Contains(name, normalized) && !strings.Contains(normalized, name)) {
			continue
		}
		hotels = append(hotels, domain.Hotel{
			ID:                fmt.Sprintf("amap:%s,%s", formatCoord(poi.Lng), formatCoord(poi.Lat)),
			Name:              query,
			Address:           poi.Address,
			City:              geocodeHotelCity,
			Lat:               poi.Lat,
			Lng:               poi.Lng,
			PricePerNightYuan: geocodeHotelPrice,
			PriceEstimated:    true,
			Provider:          "amap",
		})
		if len(hotels) >= maxGeocodeHotels {
			break
		}
	}
	return hotels
}

func normalizeHotelQuery(name string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '(', ')', '（', '）', '·':
			return -1
		}
		return r
	}, strings.TrimSpace(name))
}

// coarseGeocode rejects results that only locate the province or city center.
func coarseGeocode(level string) bool {
	switch level {
	case "国家", "省", "市", "区县", "开发区", "乡镇":
		return true
	}
	return false
}

func formatCoord(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
