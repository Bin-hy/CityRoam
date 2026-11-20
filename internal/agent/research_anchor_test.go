package agent

import (
	"testing"

	"trippilot/internal/domain"
)

func anchorTestInput(anchorDays []int) ResearchInput {
	return ResearchInput{
		RunID:  "run-anchor",
		TripID: "trip-anchor",
		Input:  "北京两天，住全季酒店",
		Constraints: domain.ConstraintSet{
			City:    "beijing",
			Days:    2,
			Lodging: domain.LodgingConstraint{Mentioned: true, Hotels: []string{"全季酒店"}, AnchorDays: anchorDays},
		},
	}
}

func anchorTestSession() *researchSession {
	return &researchSession{
		hotelAvailable: true,
		hotels: []domain.Hotel{{
			ID: "h1", Name: "全季酒店", Lat: 39.91, Lng: 116.47, PricePerNightYuan: 400, Provider: "flyai",
		}},
	}
}

func TestResearchAnchorDays(t *testing.T) {
	hotel := anchorTestSession().hotels[0]

	t.Run("explicit constraint wins over the llm proposal", func(t *testing.T) {
		in := anchorTestInput([]int{1})
		brief := domain.ResearchBrief{Accommodation: &domain.AccommodationBrief{
			Selected: &hotel, AnchorDays: []int{0},
		}}
		question := normalizeResearchBrief(&brief, in, anchorTestSession())
		if question != nil {
			t.Fatalf("question = %+v", question)
		}
		if len(brief.Accommodation.AnchorDays) != 1 || brief.Accommodation.AnchorDays[0] != 1 {
			t.Fatalf("anchor days = %+v", brief.Accommodation.AnchorDays)
		}
	})

	t.Run("defaults to every day when the constraint names no days", func(t *testing.T) {
		in := anchorTestInput(nil)
		brief := domain.ResearchBrief{Accommodation: &domain.AccommodationBrief{
			Selected: &hotel, AnchorDays: []int{0},
		}}
		normalizeResearchBrief(&brief, in, anchorTestSession())
		days := brief.Accommodation.AnchorDays
		if len(days) != 2 || days[0] != 0 || days[1] != 1 {
			t.Fatalf("anchor days = %+v", days)
		}
	})

	t.Run("defaults to every day", func(t *testing.T) {
		in := anchorTestInput(nil)
		brief := domain.ResearchBrief{Accommodation: &domain.AccommodationBrief{Selected: &hotel}}
		normalizeResearchBrief(&brief, in, anchorTestSession())
		days := brief.Accommodation.AnchorDays
		if len(days) != 2 || days[0] != 0 || days[1] != 1 {
			t.Fatalf("anchor days = %+v", days)
		}
	})

	t.Run("stays empty without a selected hotel", func(t *testing.T) {
		in := ResearchInput{Constraints: domain.ConstraintSet{City: "beijing", Days: 2}}
		brief := domain.ResearchBrief{}
		normalizeResearchBrief(&brief, in, &researchSession{})
		if brief.Accommodation != nil {
			t.Fatalf("accommodation = %+v", brief.Accommodation)
		}
	})
}

func TestResearchPriceUnknownGeocodedHotel(t *testing.T) {
	in := anchorTestInput([]int{0, 1})
	hotel := domain.Hotel{
		ID: "amap:116.46,39.92", Name: "全季酒店", Lat: 39.92, Lng: 116.46,
		Provider: "amap", PriceEstimated: true,
	}
	brief := domain.ResearchBrief{Accommodation: &domain.AccommodationBrief{Selected: &hotel}}
	session := &researchSession{hotels: []domain.Hotel{hotel}, hotelAvailable: true}

	question := normalizeResearchBrief(&brief, in, session)
	if question != nil {
		t.Fatalf("question = %+v", question)
	}
	if brief.Accommodation.Selected == nil || brief.Accommodation.Selected.Provider != "amap" {
		t.Fatalf("selected = %+v", brief.Accommodation.Selected)
	}
	if brief.Accommodation.IncludeInBudget {
		t.Fatalf("include in budget = true")
	}
	if brief.Accommodation.TotalYuan != 0 || brief.Accommodation.NightlyYuan != 0 {
		t.Fatalf("totals = %+v", brief.Accommodation)
	}
	if !containsString(brief.Warnings, "酒店价格为未知，未计入预算") {
		t.Fatalf("warnings = %v", brief.Warnings)
	}
}
