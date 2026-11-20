package runstore

import (
	"encoding/json"
	"testing"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
)

func TestEncodeVersionPlanKeepsExplanations(t *testing.T) {
	snapshot := agent.VersionSnapshot{
		Seq: 2,
		Plan: domain.Itinerary{
			City: "hangzhou",
			Days: []domain.DayPlan{{Date: "2026-11-21"}},
			Totals: domain.Totals{
				CostYuan: 680,
				POICount: 2,
			},
		},
		Explanations: []domain.Explanation{
			{POIID: "west-lake", Text: "上午光线好，适合环湖。"},
		},
	}

	encoded, err := encodeVersionPlan(snapshot)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var decoded struct {
		City         string               `json:"city"`
		Days         []domain.DayPlan     `json:"days"`
		Totals       domain.Totals        `json:"totals"`
		Explanations []domain.Explanation `json:"explanations"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.City != "hangzhou" || len(decoded.Days) != 1 || decoded.Totals.CostYuan != 680 {
		t.Fatalf("decoded = %+v", decoded)
	}
	if len(decoded.Explanations) != 1 || decoded.Explanations[0].POIID != "west-lake" {
		t.Fatalf("explanations = %+v", decoded.Explanations)
	}

	var itinerary domain.Itinerary
	if err := json.Unmarshal(encoded, &itinerary); err != nil {
		t.Fatalf("old reader decode: %v", err)
	}
	if itinerary.City != "hangzhou" || len(itinerary.Days) != 1 {
		t.Fatalf("itinerary = %+v", itinerary)
	}
}

func TestEncodeVersionPlanOmitsEmptyExplanations(t *testing.T) {
	encoded, err := encodeVersionPlan(agent.VersionSnapshot{
		Plan: domain.Itinerary{City: "hangzhou"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, exists := fields["explanations"]; exists {
		t.Fatalf("explanations should be omitted: %s", encoded)
	}
}
