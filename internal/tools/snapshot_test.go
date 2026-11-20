package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"trippilot/internal/domain"
)

func TestSnapshotSearch(t *testing.T) {
	pois := []domain.POI{
		{ID: "palace", City: "beijing", Name: "故宫", Category: "历史", Tags: []string{"must_visit"}},
		{ID: "park", City: "beijing", Name: "景山", Category: "公园", Tags: []string{"park"}},
		{ID: "food", City: "beijing", Name: "烤鸭店", Category: "餐饮", Tags: []string{"food"}},
		{ID: "street", City: "beijing", Name: "南锣鼓巷", Category: "街区", Tags: []string{"street"}},
	}
	snapshot := NewSnapshot(pois)

	t.Run("must-visit is promoted", func(t *testing.T) {
		result := snapshot.Search(SearchQuery{
			City:       "beijing",
			Categories: []string{"公园"},
			MustVisit:  []string{"故宫"},
		})
		if len(result) != 2 {
			t.Fatalf("result = %+v, want 2", result)
		}
		if result[0].ID != "palace" || result[1].ID != "park" {
			t.Fatalf("result ids = %s, %s, want palace, park", result[0].ID, result[1].ID)
		}
	})

	t.Run("tag filter", func(t *testing.T) {
		result := snapshot.Search(SearchQuery{City: "beijing", Tags: []string{"food"}})
		if len(result) != 1 || result[0].ID != "food" {
			t.Fatalf("result = %+v, want food only", result)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		result := snapshot.Search(SearchQuery{City: "beijing", Categories: []string{"不存在"}})
		if len(result) != 0 {
			t.Fatalf("result = %+v, want none", result)
		}
	})

	t.Run("other city excluded", func(t *testing.T) {
		result := snapshot.Search(SearchQuery{City: "shanghai"})
		if len(result) != 0 {
			t.Fatalf("result = %+v, want none", result)
		}
	})

	t.Run("duplicate ids collapse", func(t *testing.T) {
		withDuplicate := NewSnapshot(append(append([]domain.POI{}, pois...), pois[0]))
		result := withDuplicate.Search(SearchQuery{City: "beijing"})
		if len(result) != len(pois) {
			t.Fatalf("result = %d, want %d", len(result), len(pois))
		}
	})

	t.Run("partial must-visit name matches", func(t *testing.T) {
		museum := NewSnapshot([]domain.POI{
			{ID: "park", City: "beijing", Name: "景山公园", Category: "公园"},
			{ID: "science-museum", City: "beijing", Name: "中国科学技术馆", Category: "博物馆"},
		})
		result := museum.Search(SearchQuery{City: "beijing", MustVisit: []string{"科学技术馆"}})
		if len(result) == 0 || result[0].ID != "science-museum" {
			t.Fatalf("result = %+v, want science-museum promoted", result)
		}
	})

	t.Run("must-visit alias matches", func(t *testing.T) {
		museum := NewSnapshot([]domain.POI{
			{ID: "park", City: "beijing", Name: "景山公园", Category: "公园"},
			{ID: "science-museum", City: "beijing", Name: "中国科学技术馆", Aliases: []string{"科技馆"}, Category: "博物馆"},
		})
		result := museum.Search(SearchQuery{City: "beijing", MustVisit: []string{"科技馆"}})
		if len(result) == 0 || result[0].ID != "science-museum" {
			t.Fatalf("result = %+v, want alias promoted", result)
		}
	})
}

func TestSnapshotSearchLimit(t *testing.T) {
	pois := make([]domain.POI, 0, 70)
	for index := 0; index < 70; index++ {
		pois = append(pois, domain.POI{
			ID:       fmt.Sprintf("poi-%02d", index),
			City:     "beijing",
			Name:     fmt.Sprintf("POI %d", index),
			Category: "公园",
		})
	}
	last := len(pois) - 1
	pois[last].Tags = []string{"must_visit"}
	snapshot := NewSnapshot(pois)

	result := snapshot.Search(SearchQuery{
		City:      "beijing",
		Limit:     100,
		MustVisit: []string{pois[last].Name},
	})
	if len(result) != maxCandidates {
		t.Fatalf("result = %d, want %d", len(result), maxCandidates)
	}
	if result[0].ID != pois[last].ID {
		t.Fatalf("must-visit %s not promoted, first = %s", pois[last].ID, result[0].ID)
	}
	if _, found := snapshot.Get(pois[last].ID); !found {
		t.Fatalf("get POI %s: not found", pois[last].ID)
	}
}

func TestSnapshotTool(t *testing.T) {
	snapshot := NewSnapshot([]domain.POI{
		{ID: "palace", City: "beijing", Name: "故宫", Category: "历史"},
	})
	tool := NewSnapshotSearchTool(snapshot)
	if tool.Name() != "search_poi" {
		t.Fatalf("tool name = %q, want search_poi", tool.Name())
	}
	if !json.Valid(tool.InputSchema()) {
		t.Fatal("input schema is not valid JSON")
	}

	output, err := tool.Call(context.Background(), json.RawMessage(`{"city":"beijing"}`))
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	var result SearchResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if result.Count != 1 || result.POIs[0].ID != "palace" {
		t.Fatalf("result = %+v", result)
	}

	if _, err := tool.Call(context.Background(), json.RawMessage(`not json`)); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()
	tool := NewSnapshotSearchTool(NewSnapshot(nil))
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := registry.Register(tool); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	if _, found := registry.Get("search_poi"); !found {
		t.Fatal("registered tool not found")
	}
	names := registry.Names()
	if len(names) != 1 || names[0] != "search_poi" {
		t.Fatalf("names = %v", names)
	}
}
