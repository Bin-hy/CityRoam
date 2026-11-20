package scheduler

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"trippilot/internal/domain"
)

const (
	walkModeMaxMeters    = 1500
	estimatedTransitYuan = 2
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
	options domain.ScheduleOptions,
) (domain.Itinerary, error) {
	constraints = constraints.ApplyDefaults()
	dayStart, err := domain.ParseClock(constraints.Daily.StartTime)
	if err != nil {
		return domain.Itinerary{}, fmt.Errorf("scheduler: invalid day start: %w", err)
	}

	itinerary := domain.Itinerary{City: constraints.City}
	ctx := context.Background()
	anchorDays, hotelPOI := lodgingAnchor(options.Lodging, len(draft.Days))

	for dayIndex, draftDay := range draft.Days {
		blocks := make([]domain.Block, 0, len(draftDay.Items)*2)
		current := dayStart
		previousPOI := domain.POI{}
		hasPrevious := false
		blockSeq := 0
		visitDate, dateErr := time.Parse("2006-01-02", draftDay.Date)
		nextBlockID := func() string {
			blockSeq++
			return fmt.Sprintf("d%d-b%d", dayIndex+1, blockSeq)
		}
		_, anchored := anchorDays[dayIndex]
		anchored = anchored && len(draftDay.Items) > 0

		if anchored {
			firstPOI, found := pois[draftDay.Items[0].POIID]
			if !found {
				return domain.Itinerary{}, fmt.Errorf(
					"scheduler: day %d references unknown POI %q", dayIndex+1, draftDay.Items[0].POIID)
			}
			transport := anchorTransport(ctx, routes, hotelPOI, firstPOI)
			blocks = append(blocks, domain.Block{
				ID:          fmt.Sprintf("d%d-hotel-start", dayIndex+1),
				Type:        domain.BlockTypeTransit,
				Title:       "从酒店出发",
				Start:       domain.FormatClock(current),
				End:         domain.FormatClock(current + transport.DurationMin),
				DurationMin: transport.DurationMin,
				CostYuan:    transport.CostYuan,
				Transport:   &transport,
				Notes:       "起终点交通",
			})
			current += transport.DurationMin
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

				buffer := domain.BufferMinutes(previousPOI, poi)
				transitMinutes := transport.DurationMin + buffer
				blocks = append(blocks, domain.Block{
					ID:          nextBlockID(),
					Type:        domain.BlockTypeTransit,
					Title:       "前往 " + displayName(poi),
					Start:       domain.FormatClock(current),
					End:         domain.FormatClock(current + transitMinutes),
					DurationMin: transitMinutes,
					CostYuan:    transport.CostYuan,
					Transport:   &transport,
					Notes:       fmt.Sprintf("含 %d 分钟缓冲", buffer),
				})
				current += transitMinutes
				arrival = current
			}

			startAt := arrival
			if hint, err := domain.ParseClock(item.StartHint); err == nil && hint > startAt {
				wait := hint - startAt
				blocks = append(blocks, domain.Block{
					ID:          nextBlockID(),
					Type:        domain.BlockTypeRest,
					Title:       "等待至 " + domain.FormatClock(hint),
					Start:       domain.FormatClock(startAt),
					End:         domain.FormatClock(hint),
					DurationMin: wait,
					Notes:       "早到，等待开始时间",
				})
				startAt = hint
			}

			// Waiting for opening keeps early arrivals from becoming C1
			// violations that repair cannot fix by swapping POIs.
			if openAt, ok := nextOpening(poi, visitDate, dateErr, startAt); ok {
				blocks = append(blocks, domain.Block{
					ID:          nextBlockID(),
					Type:        domain.BlockTypeRest,
					Title:       "等待至 " + domain.FormatClock(openAt),
					Start:       domain.FormatClock(startAt),
					End:         domain.FormatClock(openAt),
					DurationMin: openAt - startAt,
					Notes:       "早于营业时间，等待开门",
				})
				startAt = openAt
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
				Lat:         poi.Lat,
				Lng:         poi.Lng,
				Title:       displayName(poi),
				Start:       domain.FormatClock(startAt),
				End:         domain.FormatClock(startAt + duration),
				DurationMin: duration,
				CostYuan:    cost,
			})
			current = startAt + duration
			previousPOI = poi
			hasPrevious = true
		}

		if anchored {
			transport := anchorTransport(ctx, routes, previousPOI, hotelPOI)
			blocks = append(blocks, domain.Block{
				ID:          fmt.Sprintf("d%d-hotel-end", dayIndex+1),
				Type:        domain.BlockTypeTransit,
				Title:       "返回酒店",
				Start:       domain.FormatClock(current),
				End:         domain.FormatClock(current + transport.DurationMin),
				DurationMin: transport.DurationMin,
				CostYuan:    transport.CostYuan,
				Transport:   &transport,
				Notes:       "起终点交通",
			})
			current += transport.DurationMin
		}

		dayEnd := domain.FormatClock(dayStart)
		if len(blocks) > 0 {
			dayEnd = blocks[len(blocks)-1].End
		}

		dayCost, poiCount, walkingMeters := summarize(blocks)
		itinerary.Days = append(itinerary.Days, domain.DayPlan{
			Date:        draftDay.Date,
			StartTime:   domain.FormatClock(dayStart),
			EndTime:     dayEnd,
			Blocks:      blocks,
			DayCostYuan: dayCost,
		})
		itinerary.Totals.CostYuan += dayCost
		itinerary.Totals.POICount += poiCount
		itinerary.Totals.WalkingKM += walkingMeters / 1000
	}

	if options.Lodging != nil {
		itinerary.Lodging = lodgingSummary(options.Lodging, len(itinerary.Days))
		itinerary.Totals.LodgingYuan = itinerary.Lodging.TotalYuan
	}

	return itinerary, nil
}

func lodgingAnchor(anchor *domain.LodgingAnchor, days int) (map[int]struct{}, domain.POI) {
	if anchor == nil {
		return nil, domain.POI{}
	}
	anchors := make(map[int]struct{})
	for _, day := range domain.NormalizeAnchorDays(anchor.AnchorDays, days) {
		anchors[day] = struct{}{}
	}
	id := anchor.Hotel.ID
	if id == "" {
		id = "hotel:" + anchor.Hotel.Name
	}
	return anchors, domain.POI{
		ID:   id,
		Name: anchor.Hotel.Name,
		City: anchor.Hotel.City,
		Lat:  anchor.Hotel.Lat,
		Lng:  anchor.Hotel.Lng,
	}
}

// anchorTransport prefers the route provider and falls back to a straight-line
// estimate so a day anchored at the hotel never fails to schedule.
func anchorTransport(ctx context.Context, routes domain.RouteProvider, from, to domain.POI) domain.Transport {
	mode := chooseMode(from, to)
	if routes != nil {
		if transport, err := routes.Route(ctx, from, to, mode); err == nil {
			normalizeTransport(&transport, from, to, mode)
			return transport
		}
	}
	distance := distanceMeters(from, to)
	speed := 80.0
	cost := 0
	if mode != domain.TransportWalk {
		speed = 250
		cost = estimatedTransitYuan
	}
	minutes := int(math.Ceil(distance / speed))
	if minutes < 1 {
		minutes = 1
	}
	return domain.Transport{
		Mode:        mode,
		FromPOIID:   from.ID,
		ToPOIID:     to.ID,
		DistanceM:   int(math.Round(distance)),
		DurationMin: minutes,
		CostYuan:    cost,
		Estimated:   true,
	}
}

func lodgingSummary(anchor *domain.LodgingAnchor, days int) *domain.LodgingSummary {
	nights := days - 1
	if nights < 0 {
		nights = 0
	}
	nightly := anchor.Hotel.PricePerNightYuan
	if nightly < 0 {
		nightly = 0
	}
	total := 0
	if nights > 0 && nightly <= int(^uint(0)>>1)/nights {
		total = nightly * nights
	}
	return &domain.LodgingSummary{
		Hotel:           anchor.Hotel,
		Nights:          nights,
		NightlyYuan:     nightly,
		TotalYuan:       total,
		AnchorDays:      domain.NormalizeAnchorDays(anchor.AnchorDays, days),
		IncludeInBudget: anchor.IncludeInBudget,
		PriceEstimated:  anchor.Hotel.PriceEstimated || nightly == 0,
		Source:          anchor.Hotel.Provider,
	}
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

// nextOpening returns the next opening minute when arrival is before the
// POI opens on the visit date. Arrivals inside a range or after the last
// closing time are left to the validator.
func nextOpening(poi domain.POI, visitDate time.Time, dateErr error, at int) (int, bool) {
	if dateErr != nil || len(poi.OpenHours) == 0 {
		return 0, false
	}
	weekday := int(visitDate.Weekday())
	next := -1
	for _, r := range poi.OpenHours {
		start, startErr := domain.ParseClock(r.Start)
		end, endErr := domain.ParseClock(r.End)
		if startErr != nil || endErr != nil || end <= start {
			continue
		}
		if !slices.Contains(r.Days, weekday) {
			continue
		}
		if at >= start && at < end {
			return 0, false
		}
		if start > at && (next == -1 || start < next) {
			next = start
		}
	}
	if next == -1 {
		return 0, false
	}
	return next, true
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
