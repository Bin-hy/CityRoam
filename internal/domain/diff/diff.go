package diff

import (
	"reflect"
	"strconv"

	"trippilot/internal/domain"
)

type Change string

const (
	Added    Change = "added"
	Removed  Change = "removed"
	Modified Change = "modified"
)

type DayChange struct {
	DayIndex int             `json:"day_index"`
	Change   Change          `json:"change"`
	Before   *domain.DayPlan `json:"before,omitempty"`
	After    *domain.DayPlan `json:"after,omitempty"`
}

type BlockChange struct {
	DayIndex int           `json:"day_index"`
	BlockID  string        `json:"block_id"`
	Change   Change        `json:"change"`
	Before   *domain.Block `json:"before,omitempty"`
	After    *domain.Block `json:"after,omitempty"`
}

type Result struct {
	Days   []DayChange   `json:"days"`
	Blocks []BlockChange `json:"blocks"`
}

// Diff compares two itinerary versions by day index and block ID.
// Unchanged days and blocks are omitted; changes keep the order they
// appear in the newer version, with removals appended after additions.
func Diff(before, after domain.Itinerary) Result {
	result := Result{
		Days:   make([]DayChange, 0),
		Blocks: make([]BlockChange, 0),
	}

	total := len(before.Days)
	if len(after.Days) > total {
		total = len(after.Days)
	}

	for dayIndex := 0; dayIndex < total; dayIndex++ {
		switch {
		case dayIndex >= len(before.Days):
			day := after.Days[dayIndex]
			result.Days = append(result.Days, DayChange{
				DayIndex: dayIndex,
				Change:   Added,
				After:    &day,
			})
		case dayIndex >= len(after.Days):
			day := before.Days[dayIndex]
			result.Days = append(result.Days, DayChange{
				DayIndex: dayIndex,
				Change:   Removed,
				Before:   &day,
			})
		default:
			beforeDay := before.Days[dayIndex]
			afterDay := after.Days[dayIndex]
			if reflect.DeepEqual(beforeDay, afterDay) {
				continue
			}
			result.Days = append(result.Days, DayChange{
				DayIndex: dayIndex,
				Change:   Modified,
				Before:   &beforeDay,
				After:    &afterDay,
			})
			result.Blocks = append(result.Blocks, diffBlocks(dayIndex, beforeDay, afterDay)...)
		}
	}
	return result
}

func diffBlocks(dayIndex int, before, after domain.DayPlan) []BlockChange {
	changes := make([]BlockChange, 0)
	beforeByKey := indexBlocks(before.Blocks)
	afterByKey := indexBlocks(after.Blocks)

	for index, block := range after.Blocks {
		current := block
		key := blockKey(index, block)
		prior, found := beforeByKey[key]
		if !found {
			changes = append(changes, BlockChange{
				DayIndex: dayIndex,
				BlockID:  block.ID,
				Change:   Added,
				After:    &current,
			})
			continue
		}
		if reflect.DeepEqual(prior, block) {
			continue
		}
		previous := prior
		changes = append(changes, BlockChange{
			DayIndex: dayIndex,
			BlockID:  block.ID,
			Change:   Modified,
			Before:   &previous,
			After:    &current,
		})
	}

	for index, block := range before.Blocks {
		if _, found := afterByKey[blockKey(index, block)]; found {
			continue
		}
		previous := block
		changes = append(changes, BlockChange{
			DayIndex: dayIndex,
			BlockID:  block.ID,
			Change:   Removed,
			Before:   &previous,
		})
	}
	return changes
}

func indexBlocks(blocks []domain.Block) map[string]domain.Block {
	indexed := make(map[string]domain.Block, len(blocks))
	for index, block := range blocks {
		indexed[blockKey(index, block)] = block
	}
	return indexed
}

func blockKey(index int, block domain.Block) string {
	if block.ID != "" {
		return block.ID
	}
	return "index:" + strconv.Itoa(index)
}
