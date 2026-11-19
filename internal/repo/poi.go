package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"trippilot/internal/domain"
)

func (s *Store) UpsertPOI(ctx context.Context, poi domain.POI) error {
	openHours, err := json.Marshal(poi.OpenHours)
	if err != nil {
		return fmt.Errorf("store: encode open hours for %s: %w", poi.ID, err)
	}

	arg := UpsertPOIParams{
		ID:          poi.ID,
		City:        poi.City,
		Name:        poi.Name,
		Category:    poi.Category,
		Area:        optionalString(poi.Area),
		Address:     optionalString(poi.Address),
		Lat:         poi.Lat,
		Lng:         poi.Lng,
		CoordSystem: poi.CoordSystem,
		OpenHours:   openHours,
		TicketYuan:  optionalInt(poi.TicketYuan),
		Rating:      poi.Rating,
		Outdoor:     poi.Outdoor,
		Tags:        poi.Tags,
		Source:      poi.Source,
		VerifiedAt:  poi.VerifiedAt,
	}
	if poi.SuggestedMinutes > 0 {
		suggested := int32(poi.SuggestedMinutes)
		arg.SuggestedMinutes = &suggested
	}
	if arg.Tags == nil {
		arg.Tags = []string{}
	}

	if err := s.q.UpsertPOI(ctx, arg); err != nil {
		return fmt.Errorf("store: upsert POI %s: %w", poi.ID, err)
	}
	return nil
}

func (s *Store) CountPOIs(ctx context.Context, city string) (int64, error) {
	return s.q.CountPOIs(ctx, city)
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalInt(value *int) *int32 {
	if value == nil {
		return nil
	}
	converted := int32(*value)
	return &converted
}
