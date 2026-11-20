package skills

import (
	"fmt"
	"sort"
	"strings"
)

// MaxCatalogBytes bounds the rendered catalog, not the raw metadata. The
// research prompt consumes exactly CatalogText, so the budget is measured on
// what the model actually sees.
const MaxCatalogBytes = 8 << 10

func CatalogLine(entry CatalogEntry) string {
	return fmt.Sprintf("- %s@%s：%s", entry.Name, entry.Version, entry.Description)
}

func CatalogLines(entries []CatalogEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, CatalogLine(entry))
	}
	return lines
}

// CatalogText is the canonical rendered catalog. A trailing newline mirrors
// the prompt template's per-line output when the catalog is non-empty.
func CatalogText(entries []CatalogEntry) string {
	lines := CatalogLines(entries)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func catalogEntries(skills []Skill) []CatalogEntry {
	if len(skills) == 0 {
		return nil
	}
	entries := make([]CatalogEntry, 0, len(skills))
	for _, skill := range skills {
		entries = append(entries, skill.CatalogEntry())
	}
	return entries
}

func ValidateCatalog(entries []CatalogEntry) error {
	text := CatalogText(entries)
	if len(text) <= MaxCatalogBytes {
		return nil
	}
	type largestEntry struct {
		label string
		size  int
	}
	largest := make([]largestEntry, 0, len(entries))
	for _, entry := range entries {
		largest = append(largest, largestEntry{
			label: entry.Name + "@" + entry.Version,
			size:  len(CatalogLine(entry)) + 1,
		})
	}
	sort.SliceStable(largest, func(i, j int) bool { return largest[i].size > largest[j].size })
	if len(largest) > 3 {
		largest = largest[:3]
	}
	parts := make([]string, 0, len(largest))
	for _, item := range largest {
		parts = append(parts, fmt.Sprintf("%s (%d bytes)", item.label, item.size))
	}
	return fmt.Errorf("skills: rendered catalog is %d bytes (limit %d); largest: %s",
		len(text), MaxCatalogBytes, strings.Join(parts, ", "))
}
