package tools

import (
	"context"
	"errors"
	"strings"

	"trippilot/internal/domain"
)

// AMapSource adapts AMap text search to the agent's CandidateSource. It is an
// enrichment path: the snapshot stays authoritative for hours and tickets.
type AMapSource struct {
	client *AMapClient
}

func NewAMapSource(client *AMapClient) *AMapSource {
	return &AMapSource{client: client}
}

func (s *AMapSource) SearchCandidates(ctx context.Context, query domain.POIQuery) ([]domain.POI, error) {
	if s.client == nil {
		return nil, errors.New("amap: source has no client")
	}

	terms := keywordTerms(query)
	if len(terms) == 0 {
		return nil, nil
	}
	found, err := s.client.SearchPOI(ctx, strings.Join(terms, "|"), query.City)
	if err != nil {
		return nil, err
	}

	pois := make([]domain.POI, 0, len(found))
	for _, item := range found {
		if item.ID == "" || strings.TrimSpace(item.Name) == "" {
			continue
		}
		pois = append(pois, domain.POI{
			ID:               "amap:" + item.ID,
			City:             query.City,
			Name:             item.Name,
			Category:         primaryCategory(item.Category),
			Address:          item.Address,
			Lat:              item.Lat,
			Lng:              item.Lng,
			CoordSystem:      domain.CoordSystemGCJ02,
			SuggestedMinutes: 90,
			Source:           "amap",
		})
	}
	return pois, nil
}

func keywordTerms(query domain.POIQuery) []string {
	seen := make(map[string]struct{})
	terms := make([]string, 0, len(query.Keywords)+len(query.MustVisit)+2)
	appendTerm := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		if _, dup := seen[trimmed]; dup {
			return
		}
		seen[trimmed] = struct{}{}
		terms = append(terms, trimmed)
	}
	for _, term := range query.MustVisit {
		appendTerm(term)
	}
	for _, term := range query.Keywords {
		appendTerm(term)
	}
	if len(terms) == 0 {
		appendTerm("景点")
		appendTerm("博物馆")
		appendTerm("公园")
	}
	return terms
}

func primaryCategory(raw string) string {
	first, _, _ := strings.Cut(raw, ";")
	return strings.TrimSpace(first)
}
