package validator

import (
	"context"
	"fmt"
	"strings"
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

func checkC3(plan domain.Itinerary, pois map[string]domain.POI, routes domain.RouteProvider) []domain.Violation {
	if routes == nil {
		return nil
	}

	violations := make([]domain.Violation, 0)
	ctx := context.Background()

	for dayIndex, day := range plan.Days {
		var previous *domain.Block
		var transit *domain.Transport

		for blockIndex := range day.Blocks {
			block := &day.Blocks[blockIndex]
			if block.Type == domain.BlockTypeTransit {
				transit = block.Transport
				continue
			}
			if block.Type != domain.BlockTypePOI && block.Type != domain.BlockTypeMeal {
				continue
			}

			if previous != nil {
				previousPOI, previousFound := pois[previous.POIID]
				currentPOI, currentFound := pois[block.POIID]
				previousEnd, previousErr := domain.ParseClock(previous.End)
				currentStart, currentErr := domain.ParseClock(block.Start)

				if previousFound && currentFound && previousErr == nil && currentErr == nil {
					gap := currentStart - previousEnd
					if gap < 0 {
						gap += domain.MinutesPerDay
					}

					mode := domain.TransportTransit
					if transit != nil && transit.Mode != "" {
						mode = transit.Mode
					}
					transport, err := routes.Route(ctx, previousPOI, currentPOI, mode)
					if err != nil {
						violations = append(violations, domain.Violation{
							Code:       "C3",
							Level:      domain.LevelWarning,
							Day:        dayIndex + 1,
							BlockID:    block.ID,
							POIID:      block.POIID,
							Message:    fmt.Sprintf("%s 到 %s 无法获取路程，未校验间隔", displayName(previousPOI), displayName(currentPOI)),
							Suggestion: "稍后重试或改为直线距离估算",
						})
					} else {
						if transport.Estimated {
							violations = append(violations, domain.Violation{
								Code:       "C3",
								Level:      domain.LevelWarning,
								Day:        dayIndex + 1,
								BlockID:    block.ID,
								POIID:      block.POIID,
								Message:    fmt.Sprintf("%s 到 %s 的路程为估算值，请预留时间", displayName(previousPOI), displayName(currentPOI)),
								Suggestion: "出发前确认实际路程",
							})
						}

						required := transport.DurationMin + domain.BufferMinutes(previousPOI, currentPOI)
						if required < 0 {
							required = 0
						}
						if gap < required {
							violations = append(violations, domain.Violation{
								Code:    "C3",
								Level:   domain.LevelBlocking,
								Day:     dayIndex + 1,
								BlockID: block.ID,
								POIID:   block.POIID,
								Message: fmt.Sprintf("%s 到 %s 间隔 %d 分钟，少于路程加缓冲 %d 分钟",
									displayName(previousPOI), displayName(currentPOI), gap, required),
								Suggestion: "增加交通时间或调整点位顺序",
							})
						}
					}
				}
			}

			previous = block
			transit = nil
		}
	}

	return violations
}

func checkC4(plan domain.Itinerary, constraints domain.ConstraintSet) []domain.Violation {
	maxMinutes := constraints.Daily.MaxHours * 60
	if maxMinutes <= 0 {
		return nil
	}

	violations := make([]domain.Violation, 0)
	for dayIndex, day := range plan.Days {
		start, startErr := domain.ParseClock(day.StartTime)
		end, endErr := domain.ParseClock(day.EndTime)
		if startErr != nil || endErr != nil {
			continue
		}

		duration := end - start
		if duration < 0 {
			duration += domain.MinutesPerDay
		}
		if duration > maxMinutes {
			violations = append(violations, domain.Violation{
				Code:       "C4",
				Level:      domain.LevelBlocking,
				Day:        dayIndex + 1,
				Message:    fmt.Sprintf("第 %d 天总时长 %d 分钟，超过上限 %d 分钟", dayIndex+1, duration, maxMinutes),
				Suggestion: "减少点位或放宽每日时长",
			})
		}
	}
	return violations
}

func checkC5(plan domain.Itinerary, constraints domain.ConstraintSet) []domain.Violation {
	if constraints.Budget.TotalYuan <= 0 || plan.Totals.CostYuan <= constraints.Budget.TotalYuan {
		return nil
	}

	level := domain.LevelWarning
	if constraints.Budget.Hard {
		level = domain.LevelBlocking
	}
	return []domain.Violation{{
		Code:       "C5",
		Level:      level,
		Message:    fmt.Sprintf("预算 %d 元，预计 %d 元，超出 %d 元", constraints.Budget.TotalYuan, plan.Totals.CostYuan, plan.Totals.CostYuan-constraints.Budget.TotalYuan),
		Suggestion: "减少点位、换免费景点或放宽预算",
	}}
}

func checkC6(plan domain.Itinerary, constraints domain.ConstraintSet) []domain.Violation {
	if !constraints.Meals.Enabled {
		return nil
	}

	lunchStart, lunchStartErr := domain.ParseClock(constraints.Meals.LunchWindow[0])
	lunchEnd, lunchEndErr := domain.ParseClock(constraints.Meals.LunchWindow[1])
	dinnerStart, dinnerStartErr := domain.ParseClock(constraints.Meals.DinnerWindow[0])
	dinnerEnd, dinnerEndErr := domain.ParseClock(constraints.Meals.DinnerWindow[1])
	if lunchStartErr != nil || lunchEndErr != nil || dinnerStartErr != nil || dinnerEndErr != nil {
		return nil
	}

	violations := make([]domain.Violation, 0)
	for dayIndex, day := range plan.Days {
		mealSeen := false
		for _, block := range day.Blocks {
			if block.Type != domain.BlockTypeMeal {
				continue
			}
			mealSeen = true

			start, err := domain.ParseClock(block.Start)
			if err != nil {
				violations = append(violations, domain.Violation{
					Code:       "C6",
					Level:      domain.LevelWarning,
					Day:        dayIndex + 1,
					BlockID:    block.ID,
					POIID:      block.POIID,
					Message:    fmt.Sprintf("%s 的时间格式无效", block.Title),
					Suggestion: "使用 HH:MM 24 小时格式",
				})
				continue
			}
			if mealOutsideWindow(start, lunchStart, lunchEnd, dinnerStart, dinnerEnd) {
				violations = append(violations, domain.Violation{
					Code:       "C6",
					Level:      domain.LevelWarning,
					Day:        dayIndex + 1,
					BlockID:    block.ID,
					POIID:      block.POIID,
					Message:    fmt.Sprintf("%s 安排在 %s，超出用餐窗口", block.Title, block.Start),
					Suggestion: "调整到午餐或晚餐时间窗内",
				})
			}
		}

		if !mealSeen {
			violations = append(violations, domain.Violation{
				Code:       "C6",
				Level:      domain.LevelBlocking,
				Day:        dayIndex + 1,
				Message:    fmt.Sprintf("第 %d 天没有餐饮安排", dayIndex+1),
				Suggestion: "至少安排午餐或晚餐",
			})
		}
	}
	return violations
}

func mealOutsideWindow(start, lunchStart, lunchEnd, dinnerStart, dinnerEnd int) bool {
	if start >= lunchStart && start <= lunchEnd {
		return false
	}
	if start >= dinnerStart && start <= dinnerEnd {
		return false
	}

	nearest := absInt(start - lunchStart)
	for _, boundary := range []int{lunchEnd, dinnerStart, dinnerEnd} {
		if distance := absInt(start - boundary); distance < nearest {
			nearest = distance
		}
	}
	return nearest > 30
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func checkC7(plan domain.Itinerary, pois map[string]domain.POI, constraints domain.ConstraintSet) []domain.Violation {
	if len(constraints.MustVisit) == 0 {
		return nil
	}

	present := make(map[string]struct{})
	for _, day := range plan.Days {
		for _, block := range day.Blocks {
			if block.POIID == "" {
				continue
			}
			present[normalizeName(block.POIID)] = struct{}{}
			if block.Title != "" {
				present[normalizeName(block.Title)] = struct{}{}
			}
			if poi, found := pois[block.POIID]; found && poi.Name != "" {
				present[normalizeName(poi.Name)] = struct{}{}
			}
		}
	}

	violations := make([]domain.Violation, 0)
	for _, required := range constraints.MustVisit {
		if _, found := present[normalizeName(required)]; found {
			continue
		}
		violations = append(violations, domain.Violation{
			Code:       "C7",
			Level:      domain.LevelBlocking,
			Message:    fmt.Sprintf("必去点 %q 不在行程中", required),
			Suggestion: "把必去点加入行程，或确认该点是否可预约",
		})
	}
	return violations
}

func checkC8(plan domain.Itinerary) []domain.Violation {
	firstDay := make(map[string]int)
	violations := make([]domain.Violation, 0)

	for dayIndex, day := range plan.Days {
		seenToday := make(map[string]struct{})
		for _, block := range day.Blocks {
			if block.POIID == "" {
				continue
			}
			if block.Type != domain.BlockTypePOI && block.Type != domain.BlockTypeMeal {
				continue
			}

			if _, duplicated := seenToday[block.POIID]; duplicated {
				violations = append(violations, domain.Violation{
					Code:       "C8",
					Level:      domain.LevelBlocking,
					Day:        dayIndex + 1,
					BlockID:    block.ID,
					POIID:      block.POIID,
					Message:    fmt.Sprintf("第 %d 天重复安排 %s", dayIndex+1, block.Title),
					Suggestion: "删除重复点位，换成其他候选",
				})
				continue
			}
			seenToday[block.POIID] = struct{}{}

			if day, found := firstDay[block.POIID]; found {
				if day == dayIndex+1 {
					continue
				}
				violations = append(violations, domain.Violation{
					Code:       "C8",
					Level:      domain.LevelBlocking,
					Day:        dayIndex + 1,
					BlockID:    block.ID,
					POIID:      block.POIID,
					Message:    fmt.Sprintf("%s 已在第 %d 天安排，不应跨天重复", block.Title, day),
					Suggestion: "删除跨天重复点位，换成其他候选",
				})
				continue
			}
			firstDay[block.POIID] = dayIndex + 1
		}
	}
	return violations
}

func checkC9(plan domain.Itinerary, constraints domain.ConstraintSet) []domain.Violation {
	minPOIs, maxPOIs := paceRange(constraints.Pace)
	violations := make([]domain.Violation, 0)

	for dayIndex, day := range plan.Days {
		count := 0
		for _, block := range day.Blocks {
			if block.Type == domain.BlockTypePOI {
				count++
			}
		}
		if count >= minPOIs && count <= maxPOIs {
			continue
		}
		violations = append(violations, domain.Violation{
			Code:       "C9",
			Level:      domain.LevelWarning,
			Day:        dayIndex + 1,
			Message:    fmt.Sprintf("第 %d 天安排 %d 个点位，%s 节奏建议 %d–%d 个", dayIndex+1, count, constraints.Pace, minPOIs, maxPOIs),
			Suggestion: "增减点位或调整出行节奏",
		})
	}
	return violations
}

func paceRange(pace domain.Pace) (int, int) {
	switch pace {
	case domain.PaceRelaxed:
		return 2, 3
	case domain.PacePacked:
		return 4, 5
	default:
		return 3, 4
	}
}

func checkC12(plan domain.Itinerary, pois map[string]domain.POI) []domain.Violation {
	violations := make([]domain.Violation, 0)

	for dayIndex, day := range plan.Days {
		for _, block := range day.Blocks {
			if block.Type != domain.BlockTypePOI || block.DurationMin <= 0 {
				continue
			}
			poi, found := pois[block.POIID]
			if !found || poi.SuggestedMinutes <= 0 {
				continue
			}

			minimum := (poi.SuggestedMinutes*3 + 4) / 5
			if block.DurationMin >= minimum {
				continue
			}
			violations = append(violations, domain.Violation{
				Code:       "C12",
				Level:      domain.LevelWarning,
				Day:        dayIndex + 1,
				BlockID:    block.ID,
				POIID:      block.POIID,
				Message:    fmt.Sprintf("%s 停留 %d 分钟，低于建议时长的 60%%", block.Title, block.DurationMin),
				Suggestion: fmt.Sprintf("至少停留 %d 分钟", minimum),
			})
		}
	}
	return violations
}

func normalizeName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func checkC10(plan domain.Itinerary, constraints domain.ConstraintSet) []domain.Violation {
	allowedStart, startErr := domain.ParseClock(constraints.Daily.StartTime)
	allowedEnd, endErr := domain.ParseClock(constraints.Daily.EndTime)
	if startErr != nil || endErr != nil {
		return nil
	}

	violations := make([]domain.Violation, 0)
	for dayIndex, day := range plan.Days {
		for _, block := range day.Blocks {
			start, err := domain.ParseClock(block.Start)
			if err != nil {
				continue
			}
			end, err := domain.ParseClock(block.End)
			if err != nil {
				continue
			}

			if start < allowedStart {
				violations = append(violations, domain.Violation{
					Code:       "C10",
					Level:      domain.LevelBlocking,
					Day:        dayIndex + 1,
					BlockID:    block.ID,
					POIID:      block.POIID,
					Message:    fmt.Sprintf("%s 安排在 %s，早于每日开始时间 %s", block.Title, block.Start, constraints.Daily.StartTime),
					Suggestion: "调整时间或放宽每日起止时间",
				})
				break
			}
			if end > allowedEnd {
				violations = append(violations, domain.Violation{
					Code:       "C10",
					Level:      domain.LevelBlocking,
					Day:        dayIndex + 1,
					BlockID:    block.ID,
					POIID:      block.POIID,
					Message:    fmt.Sprintf("%s 结束于 %s，晚于每日结束时间 %s", block.Title, block.End, constraints.Daily.EndTime),
					Suggestion: "调整时间或放宽每日起止时间",
				})
				break
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
