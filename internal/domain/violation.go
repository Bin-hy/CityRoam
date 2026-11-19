package domain

import (
	"slices"
	"strconv"
	"strings"
)

type Level string

const (
	LevelBlocking Level = "blocking"
	LevelWarning  Level = "warning"
)

type Violation struct {
	Code       string `json:"code"`
	Level      Level  `json:"level"`
	Day        int    `json:"day"`
	BlockID    string `json:"block_id,omitempty"`
	POIID      string `json:"poi_id,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

func (l Level) Valid() bool {
	return l == LevelBlocking || l == LevelWarning
}

func (v Violation) Valid() bool {
	return v.Code != "" && v.Level.Valid() && v.Message != ""
}

func HasBlocking(violations []Violation) bool {
	for _, violation := range violations {
		if violation.Level == LevelBlocking {
			return true
		}
	}
	return false
}

func CountByLevel(violations []Violation) map[Level]int {
	counts := make(map[Level]int, 2)
	for _, violation := range violations {
		counts[violation.Level]++
	}
	return counts
}

// Sort orders blocking issues first, then by C-rule number, day, and block.
func Sort(violations []Violation) {
	slices.SortStableFunc(violations, compareViolations)
}

func compareViolations(a, b Violation) int {
	if diff := levelRank(a.Level) - levelRank(b.Level); diff != 0 {
		return diff
	}

	aNumber, aKnown := violationCodeNumber(a.Code)
	bNumber, bKnown := violationCodeNumber(b.Code)
	switch {
	case aKnown && bKnown && aNumber != bNumber:
		return compareInt(aNumber, bNumber)
	case aKnown != bKnown:
		if aKnown {
			return -1
		}
		return 1
	}

	if diff := strings.Compare(a.Code, b.Code); diff != 0 {
		return diff
	}
	if diff := compareInt(a.Day, b.Day); diff != 0 {
		return diff
	}
	if diff := strings.Compare(a.BlockID, b.BlockID); diff != 0 {
		return diff
	}
	if diff := strings.Compare(a.POIID, b.POIID); diff != 0 {
		return diff
	}
	return strings.Compare(a.Message, b.Message)
}

func levelRank(level Level) int {
	if level == LevelBlocking {
		return 0
	}
	return 1
}

func violationCodeNumber(code string) (int, bool) {
	if len(code) < 2 || code[0] != 'C' {
		return 0, false
	}
	number, err := strconv.Atoi(code[1:])
	if err != nil {
		return 0, false
	}
	return number, true
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
