package domain

import (
	"strconv"
	"testing"
)

func TestViolation(t *testing.T) {
	violations := []Violation{
		{Code: "C10", Level: LevelWarning, Day: 2, BlockID: "b2", Message: "late"},
		{Code: "C2", Level: LevelBlocking, Day: 2, BlockID: "b1", Message: "closed"},
		{Code: "C1", Level: LevelBlocking, Day: 2, BlockID: "b2", Message: "closed at visit"},
		{Code: "C1", Level: LevelBlocking, Day: 1, BlockID: "b1", Message: "closed at visit"},
		{Code: "K1", Level: LevelBlocking, Day: 0, Message: "custom"},
		{Code: "C1", Level: LevelWarning, Day: 0, Message: "hours unknown"},
	}

	if !HasBlocking(violations) {
		t.Fatal("HasBlocking() = false, want true")
	}
	if HasBlocking([]Violation{{Code: "C1", Level: LevelWarning, Message: "warn"}}) {
		t.Fatal("HasBlocking() = true for warnings only")
	}

	counts := CountByLevel(violations)
	if counts[LevelBlocking] != 4 || counts[LevelWarning] != 2 {
		t.Fatalf("CountByLevel() = %v, want blocking 4 and warning 2", counts)
	}

	Sort(violations)
	want := []string{
		"C1:1:b1",
		"C1:2:b2",
		"C2:2:b1",
		"K1:0:",
		"C1:0:",
		"C10:2:b2",
	}
	if len(violations) != len(want) {
		t.Fatalf("sorted length = %d, want %d", len(violations), len(want))
	}
	for i, violation := range violations {
		got := violation.Code + ":" + strconv.Itoa(violation.Day) + ":" + violation.BlockID
		if got != want[i] {
			t.Fatalf("violations[%d] = %s, want %s (full: %+v)", i, got, want[i], violations)
		}
	}

	valid := []Violation{
		{Code: "C1", Level: LevelBlocking, Message: "closed"},
		{Code: "C6", Level: LevelWarning, Message: "late lunch"},
	}
	for _, violation := range valid {
		if !violation.Valid() {
			t.Fatalf("Valid() = false for %+v", violation)
		}
	}
	invalid := []Violation{
		{Level: LevelBlocking, Message: "missing code"},
		{Code: "C1", Level: "fatal", Message: "bad level"},
		{Code: "C1", Level: LevelWarning},
	}
	for _, violation := range invalid {
		if violation.Valid() {
			t.Fatalf("Valid() = true for %+v", violation)
		}
	}

	if got := CountByLevel(nil); len(got) != 0 {
		t.Fatalf("CountByLevel(nil) = %v, want empty", got)
	}
}

func TestViolationSortStable(t *testing.T) {
	violations := []Violation{
		{Code: "C3", Level: LevelBlocking, Day: 1, BlockID: "b", Message: "second"},
		{Code: "C3", Level: LevelBlocking, Day: 1, BlockID: "a", Message: "first"},
	}
	Sort(violations)
	if violations[0].BlockID != "a" || violations[1].BlockID != "b" {
		t.Fatalf("sort not deterministic: %+v", violations)
	}
}
