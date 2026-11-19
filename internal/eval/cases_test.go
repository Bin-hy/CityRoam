package eval

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCase(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write case: %v", err)
	}
}

func TestLoadCasesSortsAndValidates(t *testing.T) {
	dir := t.TempDir()
	writeCase(t, dir, "b.json", `{"case_id":"b","input":"北京两天","expected":{"days":2}}`)
	writeCase(t, dir, "a.json", `{"case_id":"a","input":"北京一天","expected":{"days":1}}`)

	cases, err := LoadCases(dir)
	if err != nil {
		t.Fatalf("LoadCases: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("cases = %d, want 2", len(cases))
	}
	if cases[0].CaseID != "a" || cases[1].CaseID != "b" {
		t.Fatalf("unexpected order: %+v", cases)
	}
}

func TestLoadCasesRejectsInvalid(t *testing.T) {
	tests := map[string]string{
		"missing id":    `{"input":"北京一天","expected":{"days":1}}`,
		"missing input": `{"case_id":"a","expected":{"days":1}}`,
		"bad days":      `{"case_id":"a","input":"北京五天","expected":{"days":5}}`,
		"bad json":      `{`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeCase(t, dir, "case.json", body)
			if _, err := LoadCases(dir); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestLoadCasesRejectsDuplicates(t *testing.T) {
	dir := t.TempDir()
	writeCase(t, dir, "one.json", `{"case_id":"dup","input":"北京一天","expected":{"days":1}}`)
	writeCase(t, dir, "two.json", `{"case_id":"dup","input":"北京两天","expected":{"days":2}}`)
	if _, err := LoadCases(dir); err == nil {
		t.Fatal("expected duplicate case id error")
	}
}
