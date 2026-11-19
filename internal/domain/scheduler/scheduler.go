package scheduler

import (
	"context"
	"fmt"
	"math"
	"time"

	"trippilot/internal/domain"
)

const (
	sameAreaBufferMin    = 10
	crossAreaBufferMin   = 20
	walkModeMaxMeters    = 1500
	estimatedTransitYuan = 2
	minutesPerDay        = 24 * 60
	earthRadiusMeters    = 6371000
)

type Scheduler struct{}

func New() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) Build(
	draft domain.DraftPlan,
	constraints domain.ConstraintSet,
	pois map[string]domain.POI,
	routes domain.RouteProvider,
) (domain.Itinerary, error) {
	constraints = constraints.ApplyDefaults()
	dayStart, err := parseClock(constraints.Daily.StartTime)
	if err != nil {
		return domain.Itinerary{}, fmt.Errorf("scheduler: invalid day start: %w", err)
	}

	itinerary := domain.Itinerary{City: constraints.City}
	ctx := context.Background()

	for dayIndex, draftDay := range draft.Days {
		blocks := make([]domain.Block, 0, len(draftDay.Items)*2)
		current := dayStart
		previousPOI := domain.POI{}
		hasPrevious := false
		blockSeq := 0
		nextBlockID := func() string {
			blockSeq++
			return fmt.Sprintf("d%d-b%d", dayIndex+1, blockSeq)
		}

		for _, item := range draftDay.Items {
			poi, found := pois[item.POIID]
			if !found {
				return domain.Itinerary{}, fmt.Errorf(
					"scheduler: day %d references unknown POI %q", dayIndex+1, item.POIID)
			}

			arrival := current
			if hasPrevious {
				if routes == nil {
					return domain.Itinerary{}, fmt.Errorf(
						"scheduler: day %d needs a route provider for multi-stop days", dayIndex+1)
				}

				mode := chooseMode(previousPOI, poi)
				transport, err := routes.Route(ctx, previousPOI, poi, mode)
				if err != nil {
					return domain.Itinerary{}, fmt.Errorf(
						"scheduler: route %s -> %s: %w", previousPOI.ID, poi.ID, err)
				}
				normalizeTransport(&transport, previousPOI, poi, mode)

				buffer := bufferMinutes(previousPOI, poi)
				transitMinutes := transport.DurationMin + buffer
				blocks = append(blocks, domain.Block{
					ID:          nextBlockID(),
					Type:        domain.BlockTypeTransit,
					Title:       "前往 " + displayName(poi),
					Start:       formatClock(current),
					End:         formatClock(current + transitMinutes),
					DurationMin: transitMinutes,
					CostYuan:    transport.CostYuan,
					Transport:   &transport,
					Notes:       fmt.Sprintf("含 %d 分钟缓冲", buffer),
				})
				current += transitMinutes
				arrival = current
			}

			startAt := arrival
			if hint, err := parseClock(item.StartHint); err == nil && hint > startAt {
				wait := hint - startAt
				blocks = append(blocks, domain.Block{
					ID:          nextBlockID(),
					Type:        domain.BlockTypeRest,
					Title:       "等待至 " + formatClock(hint),
					Start:       formatClock(startAt),
					End:         formatClock(hint),
					DurationMin: wait,
					Notes:       "早到，等待开始时间",
				})
				startAt = hint
			}

			duration, err := itemDuration(item, poi)
			if err != nil {
				return domain.Itinerary{}, fmt.Errorf("scheduler: day %d: %w", dayIndex+1, err)
			}
			blockType := domain.BlockTypePOI
			cost := ticketCost(poi)
			if item.Type == domain.ItemTypeMeal {
				blockType = domain.BlockTypeMeal
				cost = constraints.Meals.BudgetPerMealYuan
			}
			blocks = append(blocks, domain.Block{
				ID:          nextBlockID(),
				Type:        blockType,
				POIID:       poi.ID,
				Title:       displayName(poi),
				Start:       formatClock(startAt),
				End:         formatClock(startAt + duration),
				DurationMin: duration,
				CostYuan:    cost,
			})
			current = startAt + duration
			previousPOI = poi
			hasPrevious = true
		}

		dayEnd := formatClock(dayStart)
		if len(blocks) > 0 {
			dayEnd = blocks[len(blocks)-1].End
		}

		dayCost, poiCount, walkingMeters := summarize(blocks)
		itinerary.Days = append(itinerary.Days, domain.DayPlan{
			Date:        draftDay.Date,
			StartTime:   formatClock(dayStart),
			EndTime:     dayEnd,
			Blocks:      blocks,
			DayCostYuan: dayCost,
		})
		itinerary.Totals.CostYuan += dayCost
		itinerary.Totals.POICount += poiCount
		itinerary.Totals.WalkingKM += walkingMeters / 1000
	}

	return itinerary, nil
}

func parseClock(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil || parsed.Format("15:04") != value {
		return 0, fmt.Errorf("%q must use HH:MM 24-hour format", value)
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

func formatClock(minutes int) string {
	minutes = ((minutes % minutesPerDay) + minutesPerDay) % minutesPerDay
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

func itemDuration(item domain.DraftItem, poi domain.POI) (int, error) {
	if item.DurationMin > 0 {
		return item.DurationMin, nil
	}
	if poi.SuggestedMinutes > 0 {
		return poi.SuggestedMinutes, nil
	}
	return 0, fmt.Errorf("POI %q has no duration", poi.ID)
}

func ticketCost(poi domain.POI) int {
	if poi.TicketYuan == nil {
		return 0
	}
	return *poi.TicketYuan
}

func displayName(poi domain.POI) string {
	if poi.Name != "" {
		return poi.Name
	}
	return poi.ID
}

func bufferMinutes(from, to domain.POI) int {
	if from.Area != "" && from.Area == to.Area {
		return sameAreaBufferMin
	}
	return crossAreaBufferMin
}

func normalizeTransport(transport *domain.Transport, from, to domain.POI, mode string) {
	if transport.Mode == "" {
		transport.Mode = mode
	}
	if transport.FromPOIID == "" {
		transport.FromPOIID = from.ID
	}
	if transport.ToPOIID == "" {
		transport.ToPOIID = to.ID
	}
	if transport.DurationMin < 0 {
		transport.DurationMin = 0
	}
	if transport.CostYuan == 0 && transport.Mode != domain.TransportWalk {
		transport.CostYuan = estimatedTransitYuan
		transport.Estimated = true
	}
}

func chooseMode(from, to domain.POI) string {
	if distanceMeters(from, to) <= walkModeMaxMeters {
		return domain.TransportWalk
	}
	return domain.TransportTransit
}

func distanceMeters(a, b domain.POI) float64 {
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLat := lat2 - lat1
	dLng := (b.Lng - a.Lng) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusMeters * math.Asin(math.Sqrt(h))
}

func summarize(blocks []domain.Block) (costYuan, poiCount int, walkingMeters float64) {
	for _, block := range blocks {
		costYuan += block.CostYuan
		if block.Type == domain.BlockTypePOI {
			poiCount++
		}
		if block.Transport != nil && block.Transport.Mode == domain.TransportWalk {
			walkingMeters += float64(block.Transport.DistanceM)
		}
	}
	return costYuan, poiCount, walkingMeters
}
