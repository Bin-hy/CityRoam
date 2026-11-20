package domain

import (
	"fmt"
	"strings"
)

// ConstraintSet is the structured interpretation of a user's request.
// City and Days are critical: ApplyDefaults never invents them.
type ConstraintSet struct {
	City      string            `json:"city"`
	Days      int               `json:"days"`
	StartDate string            `json:"start_date,omitempty"`
	PartySize int               `json:"party_size"`
	Budget    BudgetConstraint  `json:"budget"`
	Lodging   LodgingConstraint `json:"lodging"`
	Daily     DailyConstraint   `json:"daily"`
	Meals     MealConstraint    `json:"meals"`
	Pace      Pace              `json:"pace"`
	Interests []string          `json:"interests"`
	MustVisit []string          `json:"must_visit"`
	Avoid     []string          `json:"avoid"`
	Walking   WalkingConstraint `json:"walking"`
	Weather   WeatherConstraint `json:"weather"`
	Hard      []string          `json:"hard"`
	Soft      []string          `json:"soft"`
	Missing   []string          `json:"missing,omitempty"`
}

type BudgetConstraint struct {
	TotalYuan      int   `json:"total_yuan"`
	Hard           bool  `json:"hard"`
	IncludeLodging *bool `json:"include_lodging,omitempty"`
}

type LodgingConstraint struct {
	Mentioned            bool     `json:"mentioned"`
	Hotels               []string `json:"hotels,omitempty"`
	Area                 string   `json:"area,omitempty"`
	MaxPricePerNightYuan int      `json:"max_price_per_night_yuan,omitempty"`
}

// LodgingIncluded reports whether lodging belongs in the budget. A nil
// pointer means the user did not specify, so the product default applies.
func (c ConstraintSet) LodgingIncluded() bool {
	if c.Budget.IncludeLodging == nil {
		return true
	}
	return *c.Budget.IncludeLodging
}

type DailyConstraint struct {
	MaxHours  int    `json:"max_hours"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type MealConstraint struct {
	Enabled           bool      `json:"enabled"`
	LunchWindow       [2]string `json:"lunch_window"`
	DinnerWindow      [2]string `json:"dinner_window"`
	BudgetPerMealYuan int       `json:"budget_per_meal_yuan"`
}

type WalkingConstraint struct {
	MaxKMPerDay float64 `json:"max_km_per_day"`
}

type WeatherConstraint struct {
	RainAware bool `json:"rain_aware"`
}

type Pace string

const (
	PaceRelaxed Pace = "relaxed"
	PaceNormal  Pace = "normal"
	PacePacked  Pace = "packed"
)

const (
	DefaultMaxHours    = 9
	DefaultStartTime   = "09:00"
	DefaultEndTime     = "21:00"
	DefaultLunchStart  = "11:30"
	DefaultLunchEnd    = "13:30"
	DefaultDinnerStart = "17:30"
	DefaultDinnerEnd   = "19:30"
	DefaultPartySize   = 1
)

// ApplyDefaults fills non-critical fields with product defaults.
// It leaves City and Days untouched so the parser can surface missing
// critical input instead of silently inventing a trip.
func (c ConstraintSet) ApplyDefaults() ConstraintSet {
	if c.PartySize == 0 {
		c.PartySize = DefaultPartySize
	}
	if c.Pace == "" {
		c.Pace = PaceNormal
	}
	if c.Daily.MaxHours == 0 {
		c.Daily.MaxHours = DefaultMaxHours
	}
	if c.Daily.StartTime == "" {
		c.Daily.StartTime = DefaultStartTime
	}
	if c.Daily.EndTime == "" {
		c.Daily.EndTime = DefaultEndTime
	}
	if c.Meals.LunchWindow[0] == "" {
		c.Meals.LunchWindow[0] = DefaultLunchStart
	}
	if c.Meals.LunchWindow[1] == "" {
		c.Meals.LunchWindow[1] = DefaultLunchEnd
	}
	if c.Meals.DinnerWindow[0] == "" {
		c.Meals.DinnerWindow[0] = DefaultDinnerStart
	}
	if c.Meals.DinnerWindow[1] == "" {
		c.Meals.DinnerWindow[1] = DefaultDinnerEnd
	}
	return c
}

// Merge overlays the non-zero fields of patch onto base. Slices in patch
// replace their base counterparts when non-nil, which lets callers move
// constraint names between the Hard and Soft lists.
func Merge(base, patch ConstraintSet) ConstraintSet {
	out := base

	if patch.City != "" {
		out.City = patch.City
	}
	if patch.Days != 0 {
		out.Days = patch.Days
	}
	if patch.StartDate != "" {
		out.StartDate = patch.StartDate
	}
	if patch.PartySize != 0 {
		out.PartySize = patch.PartySize
	}
	if patch.Budget.TotalYuan != 0 {
		out.Budget.TotalYuan = patch.Budget.TotalYuan
	}
	if patch.Budget.Hard {
		out.Budget.Hard = true
	}
	if patch.Budget.IncludeLodging != nil {
		value := *patch.Budget.IncludeLodging
		out.Budget.IncludeLodging = &value
	}
	if patch.Lodging.Mentioned {
		out.Lodging.Mentioned = true
	}
	if patch.Lodging.Hotels != nil {
		out.Lodging.Hotels = cloneStrings(patch.Lodging.Hotels)
	}
	if patch.Lodging.Area != "" {
		out.Lodging.Area = patch.Lodging.Area
	}
	if patch.Lodging.MaxPricePerNightYuan > 0 {
		out.Lodging.MaxPricePerNightYuan = patch.Lodging.MaxPricePerNightYuan
	}
	if patch.Daily.MaxHours != 0 {
		out.Daily.MaxHours = patch.Daily.MaxHours
	}
	if patch.Daily.StartTime != "" {
		out.Daily.StartTime = patch.Daily.StartTime
	}
	if patch.Daily.EndTime != "" {
		out.Daily.EndTime = patch.Daily.EndTime
	}
	if patch.Meals.Enabled {
		out.Meals.Enabled = true
	}
	if patch.Meals.LunchWindow[0] != "" {
		out.Meals.LunchWindow[0] = patch.Meals.LunchWindow[0]
	}
	if patch.Meals.LunchWindow[1] != "" {
		out.Meals.LunchWindow[1] = patch.Meals.LunchWindow[1]
	}
	if patch.Meals.DinnerWindow[0] != "" {
		out.Meals.DinnerWindow[0] = patch.Meals.DinnerWindow[0]
	}
	if patch.Meals.DinnerWindow[1] != "" {
		out.Meals.DinnerWindow[1] = patch.Meals.DinnerWindow[1]
	}
	if patch.Meals.BudgetPerMealYuan != 0 {
		out.Meals.BudgetPerMealYuan = patch.Meals.BudgetPerMealYuan
	}
	if patch.Pace != "" {
		out.Pace = patch.Pace
	}
	if patch.Walking.MaxKMPerDay != 0 {
		out.Walking.MaxKMPerDay = patch.Walking.MaxKMPerDay
	}
	if patch.Weather.RainAware {
		out.Weather.RainAware = true
	}

	if patch.Interests != nil {
		out.Interests = cloneStrings(patch.Interests)
	}
	if patch.MustVisit != nil {
		out.MustVisit = cloneStrings(patch.MustVisit)
	}
	if patch.Avoid != nil {
		out.Avoid = cloneStrings(patch.Avoid)
	}
	if patch.Hard != nil {
		out.Hard = cloneStrings(patch.Hard)
	}
	if patch.Soft != nil {
		out.Soft = cloneStrings(patch.Soft)
	}
	if patch.Soft != nil {
		out.Hard = excludeStrings(out.Hard, patch.Soft)
	}
	if patch.Hard != nil {
		out.Soft = excludeStrings(out.Soft, patch.Hard)
	}
	if patch.Missing != nil {
		out.Missing = cloneStrings(patch.Missing)
	}

	return out
}

func (c ConstraintSet) MissingCritical() []string {
	missing := make([]string, 0, 2)
	if strings.TrimSpace(c.City) == "" {
		missing = append(missing, "city")
	}
	if c.Days == 0 {
		missing = append(missing, "days")
	}
	return missing
}

func (c ConstraintSet) Validate() error {
	if c.Days < 1 || c.Days > 4 {
		return fmt.Errorf("days must be between 1 and 4")
	}
	if c.Budget.TotalYuan < 0 {
		return fmt.Errorf("budget total_yuan must be non-negative")
	}

	clocks := []struct {
		name  string
		value string
	}{
		{"daily.start_time", c.Daily.StartTime},
		{"daily.end_time", c.Daily.EndTime},
		{"meals.lunch_window[0]", c.Meals.LunchWindow[0]},
		{"meals.lunch_window[1]", c.Meals.LunchWindow[1]},
		{"meals.dinner_window[0]", c.Meals.DinnerWindow[0]},
		{"meals.dinner_window[1]", c.Meals.DinnerWindow[1]},
	}
	for _, clock := range clocks {
		if !isClock(clock.value) {
			return fmt.Errorf("%s must use HH:MM 24-hour format", clock.name)
		}
	}
	return nil
}

func isClock(value string) bool {
	_, err := ParseClock(value)
	return err == nil
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func excludeStrings(values, remove []string) []string {
	if len(values) == 0 {
		return values
	}
	set := make(map[string]struct{}, len(remove))
	for _, name := range remove {
		if name != "" {
			set[name] = struct{}{}
		}
	}
	if len(set) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	for _, name := range values {
		if _, found := set[name]; !found {
			out = append(out, name)
		}
	}
	return out
}
