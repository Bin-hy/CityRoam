package tools

import (
	"context"
	"math"
	"net/url"
	"strconv"

	"trippilot/internal/domain"
)

const (
	walkSpeedKMH    = 4.0
	transitSpeedKMH = 15.0
	estimateBuffer  = 1.2
	earthRadiusM    = 6371000.0
)

type RouteService struct {
	amap *AMapClient
	city string
}

func NewRouteService(amap *AMapClient, city string) *RouteService {
	return &RouteService{amap: amap, city: city}
}

// Route asks AMap for a route and falls back to a straight-line estimate with
// a 20% buffer whenever the provider is unavailable, over quota, or slow.
func (s *RouteService) Route(ctx context.Context, from, to domain.POI, mode string) (domain.Transport, error) {
	if s.amap != nil {
		transport, err := s.fetch(ctx, from, to, mode)
		if err == nil {
			return transport, nil
		}
	}
	return estimateRoute(from, to, mode), nil
}

func (s *RouteService) fetch(ctx context.Context, from, to domain.POI, mode string) (domain.Transport, error) {
	params := url.Values{}
	params.Set("origin", formatLocation(from.Lng, from.Lat))
	params.Set("destination", formatLocation(to.Lng, to.Lat))

	path := "/v3/direction/walking"
	switch mode {
	case domain.TransportWalk:
		path = "/v3/direction/walking"
	case domain.TransportDrive:
		path = "/v3/direction/driving"
	default:
		path = "/v3/direction/transit/integrated"
		params.Set("city", s.city)
		params.Set("cityd", s.city)
	}

	var payload struct {
		Route struct {
			Paths []struct {
				Duration string `json:"duration"`
				Distance string `json:"distance"`
				Tolls    string `json:"tolls"`
			} `json:"paths"`
			Transits []struct {
				Duration string `json:"duration"`
				Distance string `json:"distance"`
				Cost     string `json:"cost"`
			} `json:"transits"`
		} `json:"route"`
	}
	if err := s.amap.get(ctx, path, params, &payload); err != nil {
		return domain.Transport{}, err
	}

	transport := domain.Transport{
		Mode:      mode,
		FromPOIID: from.ID,
		ToPOIID:   to.ID,
	}
	if len(payload.Route.Paths) > 0 {
		path := payload.Route.Paths[0]
		transport.DurationMin = minutesFromSeconds(path.Duration)
		transport.DistanceM = meters(path.Distance)
		if mode == domain.TransportDrive {
			transport.CostYuan = yuan(path.Tolls)
		}
		return transport, nil
	}
	if len(payload.Route.Transits) > 0 {
		transit := payload.Route.Transits[0]
		transport.DurationMin = minutesFromSeconds(transit.Duration)
		transport.DistanceM = meters(transit.Distance)
		transport.CostYuan = yuan(transit.Cost)
		return transport, nil
	}
	return domain.Transport{}, &APIError{InfoCode: "no_route", Info: "no route returned"}
}

func estimateRoute(from, to domain.POI, mode string) domain.Transport {
	distance := straightLineMeters(from, to)
	speed := walkSpeedKMH
	cost := 0
	if mode == domain.TransportTransit {
		speed = transitSpeedKMH
		cost = 2
	}
	hours := distance / 1000 / speed * estimateBuffer
	duration := int(math.Ceil(hours * 60))
	if duration < 1 {
		duration = 1
	}
	return domain.Transport{
		Mode:        mode,
		FromPOIID:   from.ID,
		ToPOIID:     to.ID,
		DurationMin: duration,
		DistanceM:   int(math.Round(distance)),
		CostYuan:    cost,
		Estimated:   true,
	}
}

func straightLineMeters(a, b domain.POI) float64 {
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLat := lat2 - lat1
	dLng := (b.Lng - a.Lng) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusM * math.Asin(math.Sqrt(h))
}

func formatLocation(lng, lat float64) string {
	return strconv.FormatFloat(lng, 'f', 6, 64) + "," + strconv.FormatFloat(lat, 'f', 6, 64)
}

func minutesFromSeconds(raw string) int {
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return int(math.Ceil(seconds / 60))
}

func meters(raw string) int {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return int(math.Round(value))
}

func yuan(raw string) int {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return int(math.Round(value))
}

var _ domain.RouteProvider = (*RouteService)(nil)
