package domain

import (
	"sort"
	"time"
)

type ResearchStatus string

const (
	ResearchOK       ResearchStatus = "ok"
	ResearchDegraded ResearchStatus = "degraded"
	ResearchSkipped  ResearchStatus = "skipped"
)

type ResearchBrief struct {
	Status        ResearchStatus      `json:"status"`
	Accommodation *AccommodationBrief `json:"accommodation,omitempty"`
	Guides        []GuideSignal       `json:"guides,omitempty"`
	Notes         []string            `json:"notes,omitempty"`
	Warnings      []string            `json:"warnings,omitempty"`
	ToolCalls     int                 `json:"tool_calls"`
	DurationMS    int64               `json:"duration_ms"`
}

type AccommodationBrief struct {
	Mentioned       bool     `json:"mentioned"`
	Specified       []string `json:"specified,omitempty"`
	Selected        *Hotel   `json:"selected,omitempty"`
	Options         []Hotel  `json:"options,omitempty"`
	AnchorDays      []int    `json:"anchor_days,omitempty"`
	Nights          int      `json:"nights"`
	NightlyYuan     int      `json:"nightly_yuan"`
	TotalYuan       int      `json:"total_yuan"`
	PriceEstimated  bool     `json:"price_estimated"`
	IncludeInBudget bool     `json:"include_in_budget"`
	Reason          string   `json:"reason,omitempty"`
}

type Hotel struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Address           string    `json:"address,omitempty"`
	City              string    `json:"city,omitempty"`
	Lat               float64   `json:"lat"`
	Lng               float64   `json:"lng"`
	Star              int       `json:"star,omitempty"`
	Score             *float64  `json:"score,omitempty"`
	PricePerNightYuan int       `json:"price_per_night_yuan"`
	PriceEstimated    bool      `json:"price_estimated"`
	BookingURL        string    `json:"booking_url,omitempty"`
	Provider          string    `json:"provider"`
	FetchedAt         time.Time `json:"fetched_at"`
}

type GuideSignal struct {
	POIID     string          `json:"poi_id"`
	Weight    float64         `json:"weight"`
	Mentions  int             `json:"mentions"`
	Note      string          `json:"note,omitempty"`
	Citations []GuideCitation `json:"citations,omitempty"`
}

type GuideCitation struct {
	Title       string     `json:"title"`
	URL         string     `json:"url"`
	Site        string     `json:"site,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	Authority   int        `json:"authority"`
}

type ResearchQuestion struct {
	Code    string           `json:"code"`
	Message string           `json:"message"`
	Options []ResearchOption `json:"options"`
}

type ResearchOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Hotel       *Hotel `json:"hotel,omitempty"`
}

func NormalizeAnchorDays(days []int, tripDays int) []int {
	if len(days) == 0 || tripDays <= 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(days))
	out := make([]int, 0, len(days))
	for _, day := range days {
		if day < 0 || day >= tripDays {
			continue
		}
		if _, found := seen[day]; found {
			continue
		}
		seen[day] = struct{}{}
		out = append(out, day)
	}
	sort.Ints(out)
	return out
}

func LodgingNights(tripDays int) int {
	if tripDays <= 0 {
		return 0
	}
	return tripDays - 1
}
