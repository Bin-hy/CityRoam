package domain

import (
	"sort"
	"time"
)

const CoordSystemGCJ02 = "GCJ-02"

type POI struct {
	ID               string      `json:"id"`
	City             string      `json:"city"`
	Name             string      `json:"name"`
	Category         string      `json:"category"`
	Area             string      `json:"area"`
	Address          string      `json:"address"`
	Lat              float64     `json:"lat"`
	Lng              float64     `json:"lng"`
	CoordSystem      string      `json:"coord_system"`
	OpenHours        []OpenRange `json:"open_hours"`
	TicketYuan       *int        `json:"ticket_yuan,omitempty"`
	SuggestedMinutes int         `json:"suggested_minutes"`
	Rating           *float64    `json:"rating,omitempty"`
	Outdoor          bool        `json:"outdoor"`
	Tags             []string    `json:"tags"`
	Source           string      `json:"source"`
	VerifiedAt       *time.Time  `json:"verified_at,omitempty"`
}

// OpenRange uses weekday numbers matching time.Weekday: 0 is Sunday.
type OpenRange struct {
	Days  []int  `json:"days"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type OpenStatus int

const (
	OpenStatusUnknown OpenStatus = iota
	OpenStatusOpen
	OpenStatusClosed
)

// OpenStatusAt reports whether the POI is open for entry at the given
// weekday and time. Empty or malformed open hours are unknown rather
// than closed so validators can downgrade them to warnings.
func (p POI) OpenStatusAt(day int, at time.Time) OpenStatus {
	if len(p.OpenHours) == 0 {
		return OpenStatusUnknown
	}

	atMinutes := at.Hour()*60 + at.Minute()
	hasValidRange := false
	for _, r := range p.OpenHours {
		start, startOK := clockMinutes(r.Start)
		end, endOK := clockMinutes(r.End)
		if !startOK || !endOK || end <= start {
			continue
		}
		hasValidRange = true
		if !containsDay(r.Days, day) {
			continue
		}
		if atMinutes >= start && atMinutes < end {
			return OpenStatusOpen
		}
	}
	if !hasValidRange {
		return OpenStatusUnknown
	}
	return OpenStatusClosed
}

func (p POI) IsOpen(day int, at time.Time) bool {
	return p.OpenStatusAt(day, at) == OpenStatusOpen
}

// OpenDays returns the weekdays covered by valid open ranges, sorted and
// deduplicated. It returns nil when no usable hours are available.
func (p POI) OpenDays() []int {
	set := make(map[int]struct{})
	for _, r := range p.OpenHours {
		if !isClock(r.Start) || !isClock(r.End) {
			continue
		}
		for _, day := range r.Days {
			if day >= 0 && day <= 6 {
				set[day] = struct{}{}
			}
		}
	}
	if len(set) == 0 {
		return nil
	}
	days := make([]int, 0, len(set))
	for day := range set {
		days = append(days, day)
	}
	sort.Ints(days)
	return days
}

func containsDay(days []int, day int) bool {
	for _, candidate := range days {
		if candidate == day {
			return true
		}
	}
	return false
}

func clockMinutes(value string) (int, bool) {
	minutes, err := ParseClock(value)
	return minutes, err == nil
}
