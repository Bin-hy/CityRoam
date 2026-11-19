package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Case is one reproducible planning scenario loaded from eval/cases.
type Case struct {
	CaseID   string   `json:"case_id"`
	Input    string   `json:"input"`
	Tags     []string `json:"tags,omitempty"`
	Expected Expected `json:"expected"`
}

// Expected lists the observable properties a passing plan must have.
type Expected struct {
	Days      int      `json:"days"`
	HardRules []string `json:"hard_rules,omitempty"`
	MustVisit []string `json:"must_visit,omitempty"`
	Avoid     []string `json:"avoid,omitempty"`
}

// LoadCases reads every *.json file in dir sorted by case id.
func LoadCases(dir string) ([]Case, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("eval: glob cases: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("eval: no cases found in %s", dir)
	}

	cases := make([]Case, 0, len(matches))
	seen := make(map[string]string, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("eval: read %s: %w", path, err)
		}
		var item Case
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("eval: decode %s: %w", path, err)
		}
		if err := item.validate(); err != nil {
			return nil, fmt.Errorf("eval: %s: %w", path, err)
		}
		if previous, dup := seen[item.CaseID]; dup {
			return nil, fmt.Errorf("eval: duplicate case_id %q in %s and %s", item.CaseID, previous, path)
		}
		seen[item.CaseID] = path
		cases = append(cases, item)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].CaseID < cases[j].CaseID })
	return cases, nil
}

func (c Case) validate() error {
	if strings.TrimSpace(c.CaseID) == "" {
		return fmt.Errorf("case_id is required")
	}
	if strings.TrimSpace(c.Input) == "" {
		return fmt.Errorf("input is required")
	}
	if c.Expected.Days < 1 || c.Expected.Days > 4 {
		return fmt.Errorf("expected.days must be between 1 and 4")
	}
	return nil
}

// RepoCase converts the file shape into the storage-facing row.
func (c Case) RepoCase() (id, input string, expected json.RawMessage, tags []string, err error) {
	expected, err = json.Marshal(c.Expected)
	if err != nil {
		return "", "", nil, nil, fmt.Errorf("eval: encode expected for %s: %w", c.CaseID, err)
	}
	return c.CaseID, c.Input, expected, c.Tags, nil
}
