package domain

import (
	"sort"
	"strings"
	"time"
)

const CoordSystemGCJ02 = "GCJ-02"

type POI struct {
	ID               string      `json:"id"`
	City             string      `json:"city"`
	Name             string      `json:"name"`
	Aliases          []string    `json:"aliases,omitempty"`
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

// NameMatches reports whether wanted refers to the POI by id, full name,
// alias or a partial name such as the shorthand users type.
func (poi POI) NameMatches(wanted string) bool {
	normalized := strings.TrimSpace(wanted)
	if normalized == "" {
		return false
	}
	if poi.ID == normalized {
		return true
	}
	candidates := make([]string, 0, len(poi.Aliases)+1)
	candidates = append(candidates, poi.Name)
	candidates = append(candidates, poi.Aliases...)
	for _, name := range candidates {
		if name == "" {
			continue
		}
		if name == normalized {
			return true
		}
		if len([]rune(name)) >= 2 && strings.Contains(name, normalized) {
			return true
		}
	}
	return false
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

// POIQuery is the candidate search contract shared by the snapshot, AMap and
// future MCP sources.
type POIQuery struct {
	City       string   `json:"city"`
	Keywords   []string `json:"keywords,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	MustVisit  []string `json:"must_visit,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}
