package budget

import "trippilot/internal/domain"

// Breakdown splits spend into tickets, meals, and transit.
type Breakdown struct {
	TicketsYuan int `json:"tickets_yuan"`
	MealsYuan   int `json:"meals_yuan"`
	TransitYuan int `json:"transit_yuan"`
	TotalYuan   int `json:"total_yuan"`
}

type Day struct {
	Date string `json:"date"`
	Breakdown
}

type Summary struct {
	Days []Day `json:"days"`
	Breakdown
}

// Summarize aggregates block costs by day and category. Rest blocks carry
// no cost and are ignored, so categories always add up to the total.
func Summarize(plan domain.Itinerary) Summary {
	summary := Summary{Days: make([]Day, 0, len(plan.Days))}
	for _, day := range plan.Days {
		daySummary := Day{Date: day.Date}
		for _, block := range day.Blocks {
			switch block.Type {
			case domain.BlockTypePOI:
				daySummary.TicketsYuan += block.CostYuan
			case domain.BlockTypeMeal:
				daySummary.MealsYuan += block.CostYuan
			case domain.BlockTypeTransit:
				daySummary.TransitYuan += block.CostYuan
			default:
				continue
			}
			daySummary.TotalYuan += block.CostYuan
		}
		summary.add(daySummary.Breakdown)
		summary.Days = append(summary.Days, daySummary)
	}
	return summary
}

func (b *Breakdown) add(other Breakdown) {
	b.TicketsYuan += other.TicketsYuan
	b.MealsYuan += other.MealsYuan
	b.TransitYuan += other.TransitYuan
	b.TotalYuan += other.TotalYuan
}
