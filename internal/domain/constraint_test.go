package domain

import "testing"

func TestConstraintDefaults(t *testing.T) {
	got := ConstraintSet{}.ApplyDefaults()

	if got.City != "" {
		t.Fatalf("City = %q, want empty", got.City)
	}
	if got.Days != 0 {
		t.Fatalf("Days = %d, want 0", got.Days)
	}
	if got.PartySize != DefaultPartySize {
		t.Fatalf("PartySize = %d, want %d", got.PartySize, DefaultPartySize)
	}
	if got.Pace != PaceNormal {
		t.Fatalf("Pace = %q, want %q", got.Pace, PaceNormal)
	}
	if got.Daily.MaxHours != DefaultMaxHours {
		t.Fatalf("MaxHours = %d, want %d", got.Daily.MaxHours, DefaultMaxHours)
	}
	if got.Daily.StartTime != DefaultStartTime || got.Daily.EndTime != DefaultEndTime {
		t.Fatalf("Daily window = %s-%s, want %s-%s",
			got.Daily.StartTime, got.Daily.EndTime, DefaultStartTime, DefaultEndTime)
	}
	if got.Meals.LunchWindow != [2]string{DefaultLunchStart, DefaultLunchEnd} {
		t.Fatalf("LunchWindow = %v, want [%s %s]", got.Meals.LunchWindow, DefaultLunchStart, DefaultLunchEnd)
	}
	if got.Meals.DinnerWindow != [2]string{DefaultDinnerStart, DefaultDinnerEnd} {
		t.Fatalf("DinnerWindow = %v, want [%s %s]", got.Meals.DinnerWindow, DefaultDinnerStart, DefaultDinnerEnd)
	}

	explicit := ConstraintSet{
		City:      "beijing",
		Days:      3,
		PartySize: 4,
		Pace:      PacePacked,
		Daily: DailyConstraint{
			MaxHours:  6,
			StartTime: "10:00",
			EndTime:   "20:00",
		},
		Meals: MealConstraint{
			LunchWindow:  [2]string{"12:00", "14:00"},
			DinnerWindow: [2]string{"18:00", "20:00"},
		},
	}
	got = explicit.ApplyDefaults()
	if got.City != "beijing" || got.Days != 3 || got.PartySize != 4 || got.Pace != PacePacked {
		t.Fatalf("explicit top-level fields were overwritten: %+v", got)
	}
	if got.Daily.MaxHours != 6 || got.Daily.StartTime != "10:00" || got.Daily.EndTime != "20:00" {
		t.Fatalf("explicit daily fields were overwritten: %+v", got.Daily)
	}
	if got.Meals.LunchWindow != [2]string{"12:00", "14:00"} {
		t.Fatalf("explicit lunch window was overwritten: %v", got.Meals.LunchWindow)
	}
	if got.Meals.DinnerWindow != [2]string{"18:00", "20:00"} {
		t.Fatalf("explicit dinner window was overwritten: %v", got.Meals.DinnerWindow)
	}
}

func TestConstraintMerge(t *testing.T) {
	base := ConstraintSet{
		City:      "beijing",
		Days:      2,
		PartySize: 2,
		Pace:      PaceRelaxed,
		Budget:    BudgetConstraint{TotalYuan: 800, Hard: true},
		Daily:     DailyConstraint{MaxHours: 8, StartTime: "10:00", EndTime: "20:00"},
		Interests: []string{"history"},
		MustVisit: []string{"forbidden-city"},
		Hard:      []string{"budget"},
		Soft:      []string{"pace"},
	}
	patch := ConstraintSet{
		Days:      3,
		Pace:      PacePacked,
		Budget:    BudgetConstraint{TotalYuan: 1200},
		Daily:     DailyConstraint{StartTime: "09:30"},
		Interests: []string{"coffee"},
		Hard:      []string{"pace"},
		Soft:      []string{"budget"},
	}

	got := Merge(base, patch)

	if got.City != "beijing" {
		t.Fatalf("City = %q, want beijing", got.City)
	}
	if got.Days != 3 {
		t.Fatalf("Days = %d, want 3", got.Days)
	}
	if got.PartySize != 2 {
		t.Fatalf("PartySize = %d, want 2", got.PartySize)
	}
	if got.Pace != PacePacked {
		t.Fatalf("Pace = %q, want %q", got.Pace, PacePacked)
	}
	if got.Budget.TotalYuan != 1200 || !got.Budget.Hard {
		t.Fatalf("Budget = %+v, want 1200 and hard", got.Budget)
	}
	if got.Daily.MaxHours != 8 || got.Daily.StartTime != "09:30" || got.Daily.EndTime != "20:00" {
		t.Fatalf("Daily = %+v, want partial merge", got.Daily)
	}
	if len(got.Interests) != 1 || got.Interests[0] != "coffee" {
		t.Fatalf("Interests = %v, want [coffee]", got.Interests)
	}
	if len(got.MustVisit) != 1 || got.MustVisit[0] != "forbidden-city" {
		t.Fatalf("MustVisit = %v, want preserved", got.MustVisit)
	}
	if len(got.Hard) != 1 || got.Hard[0] != "pace" {
		t.Fatalf("Hard = %v, want [pace]", got.Hard)
	}
	if len(got.Soft) != 1 || got.Soft[0] != "budget" {
		t.Fatalf("Soft = %v, want [budget]", got.Soft)
	}

	base.Interests[0] = "mutated"
	if got.Interests[0] != "coffee" {
		t.Fatalf("merge aliased base Interests: %v", got.Interests)
	}
}

func TestConstraintMissingCritical(t *testing.T) {
	tests := []struct {
		name string
		c    ConstraintSet
		want []string
	}{
		{name: "both missing", c: ConstraintSet{}, want: []string{"city", "days"}},
		{name: "city only", c: ConstraintSet{City: "beijing"}, want: []string{"days"}},
		{name: "days only", c: ConstraintSet{Days: 2}, want: []string{"city"}},
		{name: "both present", c: ConstraintSet{City: "beijing", Days: 2}},
		{name: "blank city", c: ConstraintSet{City: "   ", Days: 2}, want: []string{"city"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.c.MissingCritical()
			if len(got) != len(tt.want) {
				t.Fatalf("MissingCritical() = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("MissingCritical() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestConstraintValidate(t *testing.T) {
	valid := ConstraintSet{City: "beijing", Days: 2}.ApplyDefaults()

	tests := []struct {
		name    string
		mutate  func(*ConstraintSet)
		wantErr bool
	}{
		{name: "valid"},
		{name: "zero days", mutate: func(c *ConstraintSet) { c.Days = 0 }, wantErr: true},
		{name: "too many days", mutate: func(c *ConstraintSet) { c.Days = 5 }, wantErr: true},
		{name: "negative days", mutate: func(c *ConstraintSet) { c.Days = -1 }, wantErr: true},
		{name: "negative budget", mutate: func(c *ConstraintSet) { c.Budget.TotalYuan = -1 }, wantErr: true},
		{name: "unpadded start", mutate: func(c *ConstraintSet) { c.Daily.StartTime = "9:00" }, wantErr: true},
		{name: "invalid end", mutate: func(c *ConstraintSet) { c.Daily.EndTime = "24:00" }, wantErr: true},
		{name: "invalid lunch", mutate: func(c *ConstraintSet) { c.Meals.LunchWindow[1] = "13:60" }, wantErr: true},
		{name: "invalid dinner", mutate: func(c *ConstraintSet) { c.Meals.DinnerWindow[0] = "evening" }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid
			if tt.mutate != nil {
				tt.mutate(&c)
			}
			err := c.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}
