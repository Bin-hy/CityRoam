package validator

import (
	"strings"
	"testing"

	"trippilot/internal/domain"
)

func rainPOIs() map[string]domain.POI {
	return map[string]domain.POI{
		"out-1": {ID: "out-1", Outdoor: true},
		"out-2": {ID: "out-2", Outdoor: true},
		"out-3": {ID: "out-3", Outdoor: true},
		"in-1":  {ID: "in-1", Outdoor: false},
		"in-2":  {ID: "in-2", Outdoor: false},
	}
}

func rainPlan(date string, poiIDs ...string) domain.Itinerary {
	blocks := make([]domain.Block, 0, len(poiIDs))
	for _, id := range poiIDs {
		blocks = append(blocks, domain.Block{
			ID:    "b" + id,
			Type:  domain.BlockTypePOI,
			POIID: id,
			Title: id,
		})
	}
	blocks = append(blocks, domain.Block{ID: "meal", Type: domain.BlockTypeMeal, Title: "午餐"})
	return domain.Itinerary{Days: []domain.DayPlan{{Date: date, Blocks: blocks}}}
}

func rainyForecast(date string) []domain.WeatherDay {
	return []domain.WeatherDay{{Date: date, DayWeather: "小雨", NightWeather: "阴"}}
}

func TestCheckRain(t *testing.T) {
	date := "2026-12-01"

	t.Run("rainy day with mostly outdoor blocks is blocking", func(t *testing.T) {
		plan := rainPlan(date, "out-1", "out-2", "out-3", "in-1")
		violations := CheckRain(plan, rainPOIs(), rainyForecast(date))
		if len(violations) != 1 {
			t.Fatalf("violations = %+v", violations)
		}
		if violations[0].Code != "C13" || violations[0].Level != domain.LevelBlocking || violations[0].Day != 1 {
			t.Fatalf("violation = %+v", violations[0])
		}
		if !strings.Contains(violations[0].Message, "3/4") {
			t.Fatalf("message = %q", violations[0].Message)
		}
	})

	t.Run("mostly indoor blocks pass", func(t *testing.T) {
		plan := rainPlan(date, "out-1", "in-1", "in-2")
		if violations := CheckRain(plan, rainPOIs(), rainyForecast(date)); len(violations) != 0 {
			t.Fatalf("violations = %+v", violations)
		}
	})

	t.Run("sunny day passes", func(t *testing.T) {
		plan := rainPlan(date, "out-1", "out-2", "out-3", "in-1")
		forecast := []domain.WeatherDay{{Date: date, DayWeather: "晴", NightWeather: "多云"}}
		if violations := CheckRain(plan, rainPOIs(), forecast); len(violations) != 0 {
			t.Fatalf("violations = %+v", violations)
		}
	})

	t.Run("night rain counts", func(t *testing.T) {
		plan := rainPlan(date, "out-1", "out-2")
		forecast := []domain.WeatherDay{{Date: date, DayWeather: "多云", NightWeather: "雷阵雨"}}
		if violations := CheckRain(plan, rainPOIs(), forecast); len(violations) != 1 {
			t.Fatalf("violations = %+v", violations)
		}
	})

	t.Run("no forecast for the date passes", func(t *testing.T) {
		plan := rainPlan(date, "out-1", "out-2", "out-3", "in-1")
		forecast := []domain.WeatherDay{{Date: "2026-12-02", DayWeather: "小雨"}}
		if violations := CheckRain(plan, rainPOIs(), forecast); len(violations) != 0 {
			t.Fatalf("violations = %+v", violations)
		}
	})

	t.Run("unknown POIs are ignored", func(t *testing.T) {
		plan := rainPlan(date, "out-1", "in-1", "missing-1", "missing-2")
		if violations := CheckRain(plan, rainPOIs(), rainyForecast(date)); len(violations) != 0 {
			t.Fatalf("violations = %+v", violations)
		}
	})

	t.Run("day without POI blocks passes", func(t *testing.T) {
		plan := domain.Itinerary{Days: []domain.DayPlan{{
			Date:   date,
			Blocks: []domain.Block{{ID: "meal", Type: domain.BlockTypeMeal, Title: "午餐"}},
		}}}
		if violations := CheckRain(plan, rainPOIs(), rainyForecast(date)); len(violations) != 0 {
			t.Fatalf("violations = %+v", violations)
		}
	})
}
