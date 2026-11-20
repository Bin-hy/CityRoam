package skills

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	LintBodyTooLong       = "body_too_long"
	LintMissingTriggerCue = "missing_trigger_cue"
	MaxLintBodyLines      = 500
)

var englishTriggerCue = regexp.MustCompile(`(?i)(?:^|[^a-z])(?:use when|when|for)(?:[^a-z]|$)`)

type LintIssue struct {
	Skill   string
	Version string
	Rule    string
	Message string
}

func (i LintIssue) String() string {
	return fmt.Sprintf("%s@%s %s: %s", i.Skill, i.Version, i.Rule, i.Message)
}

// Lint validates the pinned skills first, then reports authoring warnings.
// Warnings never fail here; the CLI decides whether -strict applies.
func Lint(dir string) ([]LintIssue, error) {
	registry, err := Load(dir)
	if err != nil {
		return nil, err
	}
	issues := make([]LintIssue, 0)
	for _, skill := range registry.Snapshot() {
		lines := strings.Count(skill.Body, "\n") + 1
		if lines > MaxLintBodyLines {
			issues = append(issues, LintIssue{
				Skill:   skill.Name,
				Version: skill.Version,
				Rule:    LintBodyTooLong,
				Message: fmt.Sprintf("body has %d lines (limit %d)", lines, MaxLintBodyLines),
			})
		}
		if !hasTriggerCue(skill.Description) {
			issues = append(issues, LintIssue{
				Skill:   skill.Name,
				Version: skill.Version,
				Rule:    LintMissingTriggerCue,
				Message: "description should state when to use the skill (当/用于/需要/适合/when/use when/for)",
			})
		}
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Skill != issues[j].Skill {
			return issues[i].Skill < issues[j].Skill
		}
		return issues[i].Rule < issues[j].Rule
	})
	return issues, nil
}

func hasTriggerCue(description string) bool {
	for _, cue := range []string{"当", "用于", "需要", "适合"} {
		if strings.Contains(description, cue) {
			return true
		}
	}
	return englishTriggerCue.MatchString(description)
}
