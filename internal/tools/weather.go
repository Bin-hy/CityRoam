package tools

import "context"

type WeatherService struct {
	amap *AMapClient
}

func NewWeatherService(amap *AMapClient) *WeatherService {
	return &WeatherService{amap: amap}
}

// Forecast is a soft dependency: any provider failure yields an empty
// forecast so planning can continue without weather context.
func (s *WeatherService) Forecast(ctx context.Context, city string) []WeatherDay {
	if s.amap == nil {
		return []WeatherDay{}
	}
	days, err := s.amap.Weather(ctx, city)
	if err != nil {
		return []WeatherDay{}
	}
	return days
}
