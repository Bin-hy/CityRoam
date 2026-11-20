package domain

import "strings"

type WeatherDay struct {
	Date         string `json:"date"`
	DayWeather   string `json:"day_weather"`
	NightWeather string `json:"night_weather"`
	DayTemp      string `json:"day_temp"`
	NightTemp    string `json:"night_temp"`
}

// IsRainy reports whether a forecast text implies precipitation.
func IsRainy(weather string) bool {
	normalized := strings.ToLower(strings.TrimSpace(weather))
	if normalized == "" {
		return false
	}
	for _, token := range []string{"雨", "雪", "rain", "snow"} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}
