package agent

import (
	"context"
	"strconv"
	"strings"

	"trippilot/internal/domain"
)

// HotelResolver turns a specified hotel name into geocoded candidates.
type HotelResolver interface {
	Resolve(ctx context.Context, query string) ([]domain.Hotel, error)
}

const maxHotelResolveCalls = 2

// resolveSpecifiedHotels geocodes specified hotel names the tool results did
// not cover. It returns the merged candidate list (existing results first)
// and the number of resolver calls made.
func resolveSpecifiedHotels(ctx context.Context, resolver HotelResolver, specified []string, existing []domain.Hotel) ([]domain.Hotel, int) {
	if resolver == nil {
		return existing, 0
	}
	names := make([]string, 0, len(specified))
	seen := make(map[string]struct{}, len(specified))
	for _, name := range specified {
		normalized := normalizeHotelName(name)
		if normalized == "" {
			continue
		}
		if _, dup := seen[normalized]; dup {
			continue
		}
		seen[normalized] = struct{}{}
		names = append(names, strings.TrimSpace(name))
	}

	resolved := make([]domain.Hotel, 0)
	calls := 0
	for _, name := range names {
		if calls >= maxHotelResolveCalls {
			break
		}
		if len(matchSpecifiedHotels(existing, []string{name})) > 0 {
			continue
		}
		calls++
		hotels, err := resolver.Resolve(ctx, name)
		if err != nil {
			continue
		}
		resolved = append(resolved, hotels...)
	}
	return mergeHotelCandidates(existing, resolved), calls
}

func mergeHotelCandidates(existing, resolved []domain.Hotel) []domain.Hotel {
	merged := make([]domain.Hotel, 0, len(existing)+len(resolved))
	byName := make(map[string]struct{}, len(existing)+len(resolved))
	byCoord := make(map[string]struct{}, len(existing)+len(resolved))
	add := func(hotel domain.Hotel) {
		name := normalizeHotelName(hotel.Name)
		if name == "" {
			return
		}
		coord := hotelCoordKey(hotel)
		if _, dup := byName[name]; dup {
			return
		}
		if coord != "" {
			if _, dup := byCoord[coord]; dup {
				return
			}
		}
		byName[name] = struct{}{}
		if coord != "" {
			byCoord[coord] = struct{}{}
		}
		merged = append(merged, sanitizeHotel(hotel))
	}
	for _, hotel := range existing {
		add(hotel)
	}
	for _, hotel := range resolved {
		add(hotel)
	}
	return merged
}

func hotelCoordKey(hotel domain.Hotel) string {
	if hotel.Lat == 0 && hotel.Lng == 0 {
		return ""
	}
	return strconv.FormatFloat(hotel.Lat, 'f', 4, 64) + "," + strconv.FormatFloat(hotel.Lng, 'f', 4, 64)
}
