package validator

import (
	"fmt"

	"trippilot/internal/domain"
)

const rainOutdoorLimit = 0.6

// CheckRain flags days with precipitation whose itinerary is mostly outdoors.
func CheckRain(plan domain.Itinerary, pois map[string]domain.POI, forecast []domain.WeatherDay) []domain.Violation {
	if len(forecast) == 0 {
		return nil
	}
	rainy := make(map[string]bool, len(forecast))
	for _, day := range forecast {
		rainy[day.Date] = domain.IsRainy(day.DayWeather) || domain.IsRainy(day.NightWeather)
	}

	violations := make([]domain.Violation, 0)
	for dayIndex, day := range plan.Days {
		isRainy, known := rainy[day.Date]
		if !known || !isRainy {
			continue
		}
		total, outdoor := 0, 0
		for _, block := range day.Blocks {
			if block.Type != domain.BlockTypePOI {
				continue
			}
			poi, found := pois[block.POIID]
			if !found {
				continue
			}
			total++
			if poi.Outdoor {
				outdoor++
			}
		}
		if total == 0 || float64(outdoor)/float64(total) <= rainOutdoorLimit {
			continue
		}
		violations = append(violations, domain.Violation{
			Code:  "C13",
			Level: domain.LevelBlocking,
			Day:   dayIndex + 1,
			Message: fmt.Sprintf("第 %d 天（%s）有降水，户外点位占 %d/%d，雨天体验较差",
				dayIndex+1, day.Date, outdoor, total),
			Suggestion: "将户外点位换成室内点位或准备雨天备选",
		})
	}
	return violations
}
