package domain

import (
	"reflect"
	"testing"
)

func TestNormalizeAnchorDays(t *testing.T) {
	tests := []struct {
		name  string
		days  []int
		total int
		want  []int
	}{
		{name: "empty", total: 3},
		{name: "dedupe and sort", days: []int{2, 0, 2, 1}, total: 3, want: []int{0, 1, 2}},
		{name: "drop out of range", days: []int{-1, 0, 3, 2}, total: 3, want: []int{0, 2}},
		{name: "zero trip days", days: []int{0}, total: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeAnchorDays(tt.days, tt.total); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("NormalizeAnchorDays(%v, %d) = %v, want %v", tt.days, tt.total, got, tt.want)
			}
		})
	}
}

func TestLodgingNights(t *testing.T) {
	tests := []struct {
		days int
		want int
	}{
		{days: 1, want: 0},
		{days: 2, want: 1},
		{days: 4, want: 3},
		{days: 0, want: 0},
		{days: -1, want: 0},
	}
	for _, tt := range tests {
		if got := LodgingNights(tt.days); got != tt.want {
			t.Fatalf("LodgingNights(%d) = %d, want %d", tt.days, got, tt.want)
		}
	}
}
