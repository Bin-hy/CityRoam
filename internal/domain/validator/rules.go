package validator

import (
	"fmt"
	"time"

	"trippilot/internal/domain"
)

func checkOpenHours(plan domain.Itinerary, pois map[string]domain.POI) []domain.Violation {
	violations := make([]domain.Violation, 0)

	for dayIndex, day := range plan.Days {
		visitDate, err := time.Parse("2006-01-02", day.Date)
		for _, block := range day.Blocks {
			if block.Type != domain.BlockTypePOI && block.Type != domain.BlockTypeMeal {
				continue
			}
			poi, found := pois[block.POIID]
			if !found {
				continue
			}

			if err != nil {
				violations = append(violations, domain.Violation{
					Code:       "C1",
					Level:      domain.LevelWarning,
					Day:        dayIndex + 1,
					BlockID:    block.ID,
					POIID:      block.POIID,
					Message:    "行程日期无效，无法校验营业时间",
					Suggestion: "补充 YYYY-MM-DD 格式的日期",
				})
				continue
			}

			minutes, err := domain.ParseClock(block.Start)
			if err != nil {
				violations = append(violations, domain.Violation{
					Code:       "C1",
					Level:      domain.LevelWarning,
					Day:        dayIndex + 1,
					BlockID:    block.ID,
					POIID:      block.POIID,
					Message:    "开始时间格式无效，无法校验营业时间",
					Suggestion: "使用 HH:MM 24 小时格式",
				})
				continue
			}

			weekday := int(visitDate.Weekday())
			openDays := poi.OpenDays()
			if len(openDays) == 0 {
				violations = append(violations, domain.Violation{
					Code:       "C1",
					Level:      domain.LevelWarning,
					Day:        dayIndex + 1,
					BlockID:    block.ID,
					POIID:      block.POIID,
					Message:    fmt.Sprintf("%s 营业时间未知，待确认", displayName(poi)),
					Suggestion: "出发前确认营业时间",
				})
				continue
			}

			if !containsDay(openDays, weekday) {
				violations = append(violations, domain.Violation{
					Code:       "C2",
					Level:      domain.LevelBlocking,
					Day:        dayIndex + 1,
					BlockID:    block.ID,
					POIID:      block.POIID,
					Message:    fmt.Sprintf("%s 在周%s闭馆", displayName(poi), weekdayName(weekday)),
					Suggestion: "换到开放日或更换地点",
				})
				continue
			}

			at := time.Date(
				visitDate.Year(), visitDate.Month(), visitDate.Day(),
				minutes/60, minutes%60, 0, 0, visitDate.Location(),
			)
			if poi.OpenStatusAt(weekday, at) == domain.OpenStatusClosed {
				violations = append(violations, domain.Violation{
					Code:       "C1",
					Level:      domain.LevelBlocking,
					Day:        dayIndex + 1,
					BlockID:    block.ID,
					POIID:      block.POIID,
					Message:    fmt.Sprintf("%s 在 %s 不开放", displayName(poi), block.Start),
					Suggestion: "调整到开放时间或更换地点",
				})
			}
		}
	}

	return violations
}

func containsDay(days []int, day int) bool {
	for _, candidate := range days {
		if candidate == day {
			return true
		}
	}
	return false
}

func weekdayName(day int) string {
	names := [...]string{"日", "一", "二", "三", "四", "五", "六"}
	if day < 0 || day >= len(names) {
		return "?"
	}
	return names[day]
}

func displayName(poi domain.POI) string {
	if poi.Name != "" {
		return poi.Name
	}
	return poi.ID
}
