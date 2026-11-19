package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestPOIOpenHours(t *testing.T) {
	poi := POI{
		ID:   "museum",
		Name: "Museum",
		OpenHours: []OpenRange{
			{Days: []int{1, 5}, Start: "09:00", End: "12:00"},
			{Days: []int{1, 5}, Start: "13:00", End: "17:00"},
		},
	}

	tests := []struct {
		name string
		day  int
		at   string
		want OpenStatus
	}{
		{name: "before open", day: 1, at: "08:59", want: OpenStatusClosed},
		{name: "at open", day: 1, at: "09:00", want: OpenStatusOpen},
		{name: "before lunch break", day: 1, at: "11:59", want: OpenStatusOpen},
		{name: "lunch break", day: 1, at: "12:00", want: OpenStatusClosed},
		{name: "after lunch break", day: 1, at: "13:00", want: OpenStatusOpen},
		{name: "before close", day: 1, at: "16:59", want: OpenStatusOpen},
		{name: "at close", day: 1, at: "17:00", want: OpenStatusClosed},
		{name: "other open day", day: 5, at: "10:00", want: OpenStatusOpen},
		{name: "closed weekday", day: 2, at: "10:00", want: OpenStatusClosed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := parseTestClock(t, tt.at)
			if got := poi.OpenStatusAt(tt.day, at); got != tt.want {
				t.Fatalf("OpenStatusAt(%d, %s) = %v, want %v", tt.day, tt.at, got, tt.want)
			}
			if got := poi.IsOpen(tt.day, at); got != (tt.want == OpenStatusOpen) {
				t.Fatalf("IsOpen(%d, %s) = %v, want %v", tt.day, tt.at, got, tt.want == OpenStatusOpen)
			}
		})
	}

	if got, want := poi.OpenDays(), []int{1, 5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenDays() = %v, want %v", got, want)
	}
}

func TestPOIOpenHoursUnknown(t *testing.T) {
	unknown := POI{ID: "unverified"}
	if got := unknown.OpenStatusAt(1, parseTestClock(t, "10:00")); got != OpenStatusUnknown {
		t.Fatalf("empty hours status = %v, want unknown", got)
	}
	if unknown.IsOpen(1, parseTestClock(t, "10:00")) {
		t.Fatal("empty hours should not be reported as open")
	}
	if days := unknown.OpenDays(); days != nil {
		t.Fatalf("OpenDays() = %v, want nil", days)
	}

	malformed := POI{
		ID:        "malformed",
		OpenHours: []OpenRange{{Days: []int{1}, Start: "9:00", End: "17:00"}},
	}
	if got := malformed.OpenStatusAt(1, parseTestClock(t, "10:00")); got != OpenStatusUnknown {
		t.Fatalf("malformed hours status = %v, want unknown", got)
	}
	if days := malformed.OpenDays(); days != nil {
		t.Fatalf("malformed OpenDays() = %v, want nil", days)
	}
}

func parseTestClock(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		t.Fatalf("parse clock %q: %v", value, err)
	}
	return time.Date(2026, time.November, 16, parsed.Hour(), parsed.Minute(), 0, 0, time.Local)
}
