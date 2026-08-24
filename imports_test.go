package lumen

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

const frameBlock = `frame f
    composition: bayesian
    decay: none
    provenance-depth: 3
    imported-decay: most_conservative
`

func TestResolveImportsTransitive(t *testing.T) {
	// A imports B imports C — all three files load, declarations merged
	dir := t.TempDir()

	writeTemp(t, dir, "c.lm", frameBlock+`
record rec-c in f
    "Record from C"
    at: "2026-01-01T00:00:00Z"
`)
	writeTemp(t, dir, "b.lm", `import "c.lm"
`+frameBlock+`
record rec-b in f
    "Record from B"
    at: "2026-01-01T00:00:00Z"
`)
	pathA := writeTemp(t, dir, "a.lm", `import "b.lm"
`+frameBlock+`
record rec-a in f
    "Record from A"
    at: "2026-01-01T00:00:00Z"
`)

	s := NewStore()
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := LoadFileWithImports(pathA, s, now); err != nil {
		t.Fatalf("LoadFileWithImports: %v", err)
	}

	// Verify records by attempting re-assert — duplicate returns error meaning it exists
	for _, id := range []string{"rec-a", "rec-b", "rec-c"} {
		err := s.Assert(&Record{ID: id, Content: "dup", Timestamp: now, Frame: "f"})
		if err == nil {
			t.Errorf("record %s not in store (duplicate assert succeeded)", id)
		}
	}
	t.Logf("Transitive imports: all 3 records (a, b, c) loaded")
}

func TestResolveImportsCycleDetection(t *testing.T) {
	// A imports B, B imports A → cycle detected
	dir := t.TempDir()

	writeTemp(t, dir, "b.lm", `import "a.lm"
`+frameBlock)
	pathA := writeTemp(t, dir, "a.lm", `import "b.lm"
`+frameBlock)

	s := NewStore()
	now := time.Now()
	err := LoadFileWithImports(pathA, s, now)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !contains(err.Error(), "cycle") {
		t.Errorf("expected 'cycle' in error, got: %v", err)
	}
	t.Logf("Cycle detected correctly: %v", err)
}

func TestResolveImportsMissingFile(t *testing.T) {
	dir := t.TempDir()
	pathA := writeTemp(t, dir, "a.lm", `import "nonexistent.lm"
`+frameBlock)

	s := NewStore()
	err := LoadFileWithImports(pathA, s, time.Now())
	if err == nil {
		t.Fatal("expected error for missing import, got nil")
	}
	t.Logf("Missing import error: %v", err)
}

func TestResolveImportsNoImports(t *testing.T) {
	// File with no imports — LoadFileWithImports behaves like LoadFile
	dir := t.TempDir()
	path := writeTemp(t, dir, "solo.lm", frameBlock+`
record solo-rec in f
    "Standalone record"
    at: "2026-01-01T00:00:00Z"
`)
	s := NewStore()
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := LoadFileWithImports(path, s, now); err != nil {
		t.Fatalf("LoadFileWithImports: %v", err)
	}
	err := s.Assert(&Record{ID: "solo-rec", Content: "dup", Timestamp: now, Frame: "f"})
	if err == nil {
		t.Error("solo-rec not in store (duplicate assert succeeded)")
	}
	t.Log("Solo file (no imports) loads correctly")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
