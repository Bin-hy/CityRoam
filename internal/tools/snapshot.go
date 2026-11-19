package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"trippilot/internal/domain"
)

const maxCandidates = 60

type Snapshot struct {
	pois []domain.POI
}

type SearchQuery struct {
	City       string   `json:"city"`
	Categories []string `json:"categories,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	MustVisit  []string `json:"must_visit,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}

type SearchResult struct {
	POIs  []domain.POI `json:"pois"`
	Count int          `json:"count"`
}

func LoadSnapshot(path string) (*Snapshot, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tools: read snapshot: %w", err)
	}
	var pois []domain.POI
	if err := json.Unmarshal(raw, &pois); err != nil {
		return nil, fmt.Errorf("tools: parse snapshot: %w", err)
	}
	return NewSnapshot(pois), nil
}

func NewSnapshot(pois []domain.POI) *Snapshot {
	return &Snapshot{pois: pois}
}

// Search returns candidates in snapshot order, with must-visit hits promoted
// to the front and always kept when the limit truncates the result.
func (s *Snapshot) Search(query SearchQuery) []domain.POI {
	limit := query.Limit
	if limit <= 0 || limit > maxCandidates {
		limit = maxCandidates
	}

	included := make([]domain.POI, 0, limit)
	seen := make(map[string]struct{}, limit)

	appendPOI := func(poi domain.POI) {
		if len(included) >= limit {
			return
		}
		if _, duplicated := seen[poi.ID]; duplicated {
			return
		}
		seen[poi.ID] = struct{}{}
		included = append(included, poi)
	}
	includedIDs := func(poi domain.POI) bool {
		_, found := seen[poi.ID]
		return found
	}

	for _, wanted := range query.MustVisit {
		for _, poi := range s.pois {
			if !matchesCity(poi, query.City) {
				continue
			}
			if !matchesName(poi, wanted) {
				continue
			}
			appendPOI(poi)
		}
	}

	for _, poi := range s.pois {
		if includedIDs(poi) || !matchesCity(poi, query.City) {
			continue
		}
		if !matchesCategory(poi, query.Categories) || !matchesTags(poi, query.Tags) {
			continue
		}
		appendPOI(poi)
	}

	return included
}

func (s *Snapshot) Get(id string) (domain.POI, bool) {
	for _, poi := range s.pois {
		if poi.ID == id {
			return poi, true
		}
	}
	return domain.POI{}, false
}

func (s *Snapshot) ByIDs(ids []string) map[string]domain.POI {
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	found := make(map[string]domain.POI, len(ids))
	for _, poi := range s.pois {
		if _, ok := wanted[poi.ID]; ok {
			found[poi.ID] = poi
		}
	}
	return found
}

func matchesCity(poi domain.POI, city string) bool {
	return city == "" || poi.City == city
}

func matchesCategory(poi domain.POI, categories []string) bool {
	if len(categories) == 0 {
		return true
	}
	for _, category := range categories {
		if poi.Category == category {
			return true
		}
	}
	return false
}

func matchesTags(poi domain.POI, tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	for _, wanted := range tags {
		for _, tag := range poi.Tags {
			if tag == wanted {
				return true
			}
		}
	}
	return false
}

func matchesName(poi domain.POI, wanted string) bool {
	normalized := strings.TrimSpace(wanted)
	if normalized == "" {
		return false
	}
	return poi.ID == normalized || poi.Name == normalized
}

const searchPOISchema = `{
  "type": "object",
  "properties": {
    "city": {"type": "string"},
    "categories": {"type": "array", "items": {"type": "string"}},
    "tags": {"type": "array", "items": {"type": "string"}},
    "must_visit": {"type": "array", "items": {"type": "string"}},
    "limit": {"type": "integer", "minimum": 1, "maximum": 60}
  },
  "required": ["city"]
}`

type SnapshotSearchTool struct {
	snapshot *Snapshot
}

func NewSnapshotSearchTool(snapshot *Snapshot) *SnapshotSearchTool {
	return &SnapshotSearchTool{snapshot: snapshot}
}

func (t *SnapshotSearchTool) Name() string {
	return "search_poi"
}

func (t *SnapshotSearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(searchPOISchema)
}

func (t *SnapshotSearchTool) Call(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
	var query SearchQuery
	if err := json.Unmarshal(input, &query); err != nil {
		return nil, fmt.Errorf("tools: decode search_poi input: %w", err)
	}
	pois := t.snapshot.Search(query)
	result, err := json.Marshal(SearchResult{POIs: pois, Count: len(pois)})
	if err != nil {
		return nil, fmt.Errorf("tools: encode search_poi output: %w", err)
	}
	return result, nil
}
