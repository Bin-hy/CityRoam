package domain

import (
	"fmt"
	"time"
)

const MinutesPerDay = 24 * 60

func ParseClock(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil || parsed.Format("15:04") != value {
		return 0, fmt.Errorf("%q must use HH:MM 24-hour format", value)
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

func FormatClock(minutes int) string {
	minutes = ((minutes % MinutesPerDay) + MinutesPerDay) % MinutesPerDay
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}
