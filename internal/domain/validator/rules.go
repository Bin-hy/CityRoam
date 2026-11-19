package validator

import (
	"context"
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
