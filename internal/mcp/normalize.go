package mcp

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"trippilot/internal/domain"
)

const defaultConfidence = 0.5

// Evidence records where a normalized claim came from so the UI can cite it.
type Evidence struct {
	Source     string  `json:"source"`
	SourceURL  string  `json:"source_url,omitempty"`
	Claim      string  `json:"claim"`
	Confidence float64 `json:"confidence"`
}

// Candidate is a POI that passed normalization. The agent may only draft with
// POIs from the candidate set.
type Candidate struct {
	POI      domain.POI `json:"poi"`
	Evidence Evidence   `json:"evidence"`
}

// Normalized keeps every evidence row but only coordinate-bearing, valid
// records become candidates.
type Normalized struct {
	Candidates []Candidate
	Evidence   []Evidence
}

type rawItem struct {
	ID          string   `json:"id"`
	POIID       string   `json:"poi_id"`
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Category    string   `json:"category"`
	Area        string   `json:"area"`
	Address     string   `json:"address"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
	Location    string   `json:"location"`
	SourceURL   string   `json:"source_url"`
	URL         string   `json:"url"`
	Claim       string   `json:"claim"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Confidence  *float64 `json:"confidence"`
}

// Normalize accepts either a JSON array or an object containing a results,
// pois, items or candidates array. Items without usable coordinates still
// produce evidence but never enter the candidate set.
func Normalize(server, tool, city string, payload json.RawMessage) (Normalized, error) {
	source := server
	if tool != "" {
		source = server + "/" + tool
	}

	items, err := decodeItems(payload)
	if err != nil {
		return Normalized{}, err
	}

	out := Normalized{
		Candidates: []Candidate{},
		Evidence:   []Evidence{},
	}
	for _, item := range items {
		name := firstNonEmpty(item.Name, item.Title)
		if name == "" {
			continue
		}

		evidence := Evidence{
			Source:     source,
			SourceURL:  firstNonEmpty(item.SourceURL, item.URL),
			Claim:      firstNonEmpty(item.Claim, item.Summary, item.Description, name),
			Confidence: defaultConfidence,
		}
		if item.Confidence != nil {
			evidence.Confidence = clamp01(*item.Confidence)
		}
		out.Evidence = append(out.Evidence, evidence)

		lat, lng, ok := coordinates(item)
		if !ok {
			continue
		}
		id := firstNonEmpty(item.POIID, item.ID)
		if id == "" {
			id = generatedID(source, name, lat, lng)
		}
		out.Candidates = append(out.Candidates, Candidate{
			POI: domain.POI{
				ID:               id,
				City:             city,
				Name:             name,
				Category:         item.Category,
				Area:             item.Area,
				Address:          item.Address,
				Lat:              lat,
				Lng:              lng,
				CoordSystem:      domain.CoordSystemGCJ02,
				SuggestedMinutes: 60,
				Source:           source,
			},
			Evidence: evidence,
		})
	}
	return out, nil
}

// Normalize normalizes the result of an allowlisted MCP call.
func (r *Result) Normalize(server, tool, city string) (Normalized, error) {
	payload, err := r.JSON()
	if err != nil {
		return Normalized{}, err
	}
	return Normalize(server, tool, city, payload)
}

func decodeItems(payload json.RawMessage) ([]rawItem, error) {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" || trimmed == "null" {
		return nil, fmt.Errorf("mcp: empty payload")
	}

	if strings.HasPrefix(trimmed, "[") {
		var items []rawItem
		if err := json.Unmarshal(payload, &items); err != nil {
			return nil, fmt.Errorf("mcp: decode results: %w", err)
		}
		return items, nil
	}

	var wrapper struct {
		Results    []rawItem `json:"results"`
		POIs       []rawItem `json:"pois"`
		Items      []rawItem `json:"items"`
		Candidates []rawItem `json:"candidates"`
	}
	if err := json.Unmarshal(payload, &wrapper); err != nil {
		return nil, fmt.Errorf("mcp: decode results: %w", err)
	}
	for _, items := range [][]rawItem{wrapper.Results, wrapper.POIs, wrapper.Items, wrapper.Candidates} {
		if len(items) > 0 {
			return items, nil
		}
	}

	var single rawItem
	if err := json.Unmarshal(payload, &single); err != nil {
		return nil, fmt.Errorf("mcp: decode result: %w", err)
	}
	if firstNonEmpty(single.Name, single.Title) == "" {
		return nil, fmt.Errorf("mcp: payload has no recognizable results")
	}
	return []rawItem{single}, nil
}

func coordinates(item rawItem) (float64, float64, bool) {
	lat, lng := item.Lat, item.Lng
	if lat == nil || lng == nil {
		lat, lng = item.Latitude, item.Longitude
	}
	if (lat == nil || lng == nil) && item.Location != "" {
		parts := strings.Split(item.Location, ",")
		if len(parts) == 2 {
			parsedLng, errLng := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			parsedLat, errLat := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if errLng == nil && errLat == nil {
				lat, lng = &parsedLat, &parsedLng
			}
		}
	}
	if lat == nil || lng == nil {
		return 0, 0, false
	}
	if math.IsNaN(*lat) || math.IsNaN(*lng) || math.IsInf(*lat, 0) || math.IsInf(*lng, 0) {
		return 0, 0, false
	}
	if math.Abs(*lat) > 90 || math.Abs(*lng) > 180 {
		return 0, 0, false
	}
	if *lat == 0 && *lng == 0 {
		return 0, 0, false
	}
	return *lat, *lng, true
}

func generatedID(source, name string, lat, lng float64) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%s|%.5f|%.5f", source, name, lat, lng)))
	return "mcp-" + hex.EncodeToString(sum[:6])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
