package skills

import (
	"strings"
	"testing"
)

func TestCatalogLine(t *testing.T) {
	entry := CatalogEntry{Name: "research-playbook", Version: "1.0.0", Description: "研究清单"}
	if got, want := CatalogLine(entry), "- research-playbook@1.0.0：研究清单"; got != want {
		t.Fatalf("CatalogLine = %q, want %q", got, want)
	}
	if got := CatalogText(nil); got != "" {
		t.Fatalf("empty catalog = %q", got)
	}
}

func TestValidateCatalogBudget(t *testing.T) {
	small := []CatalogEntry{
		{Name: "a", Version: "1.0.0", Description: "用于小目录"},
		{Name: "b", Version: "1.0.0", Description: "用于小目录"},
	}
	if err := ValidateCatalog(small); err != nil {
		t.Fatalf("small catalog rejected: %v", err)
	}

	large := make([]CatalogEntry, 0, 32)
	for index := 0; index < 32; index++ {
		large = append(large, CatalogEntry{
			Name:        "skill-" + string(rune('a'+index)),
			Version:     "1.0.0",
			Description: strings.Repeat("用", 300),
		})
	}
	err := ValidateCatalog(large)
	if err == nil {
		t.Fatal("oversized catalog accepted")
	}
	for _, want := range []string{"limit", "largest", "skill-a@1.0.0"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}
