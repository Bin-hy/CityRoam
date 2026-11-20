package domain

// DraftFromPlan rebuilds an editable draft from a finalized itinerary so
// adjustment runs and proposal rejection can reuse the same base shape.
func DraftFromPlan(plan Itinerary) DraftPlan {
	draft := DraftPlan{Days: make([]DraftDay, 0, len(plan.Days))}
	for _, day := range plan.Days {
		items := make([]DraftItem, 0, len(day.Blocks))
		for _, block := range day.Blocks {
			if block.Type == BlockTypeTransit || block.POIID == "" {
				continue
			}
			items = append(items, DraftItem{
				POIID:       block.POIID,
				Type:        block.Type,
				DurationMin: block.DurationMin,
			})
		}
		draft.Days = append(draft.Days, DraftDay{Date: day.Date, Items: items})
	}
	return draft
}
