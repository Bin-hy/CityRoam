package domain

import "testing"

func TestIsRainy(t *testing.T) {
	cases := []struct {
		weather string
		want    bool
	}{
		{"晴", false},
		{"多云", false},
		{"阴", false},
		{"小雨", true},
		{"雷阵雨", true},
		{"雨夹雪", true},
		{"小雪", true},
		{"Rain", true},
		{"snow", true},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsRainy(tc.weather); got != tc.want {
			t.Errorf("IsRainy(%q) = %v, want %v", tc.weather, got, tc.want)
		}
	}
}
