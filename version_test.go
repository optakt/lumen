package lumen

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestBeliefVersioning(t *testing.T) {
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayNone}})

	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "Original finding", Timestamp: now})
	s.Believe(&Belief{ID: "b1", Frame: "empirical", Content: "Original conclusion", Confidence: 0.8, AssertedAt: now, Derivation: []string{"r1"}})

	// Before any revision, no version history
	h, err := s.BeliefHistory("b1")
	if err != nil { t.Fatalf("BeliefHistory: %v", err) }
	if len(h) != 0 {
		t.Errorf("expected 0 versions before revision, got %d", len(h))
	}

	s.Revise("r1", "Corrected finding", 0, "correction", now.Add(time.Hour))
	s.ReAssert("b1", "Revised conclusion with lower confidence", 0.55, now.Add(2*time.Hour))

	h, err = s.BeliefHistory("b1")
	if err != nil { t.Fatalf("BeliefHistory: %v", err) }
	if len(h) != 2 {
		t.Errorf("expected 2 versions, got %d", len(h))
	}
	t.Logf("\n%s", RenderHistory("b1", h))

	if h[0].Content != "Original conclusion" {
		t.Errorf("v1 content wrong: %q", h[0].Content)
	}
	if fmt.Sprintf("%.1f", h[0].Confidence) != "0.8" {
		t.Errorf("v1 confidence wrong: %.2f", h[0].Confidence)
	}
	if h[1].State != BeliefSuspect {
		t.Errorf("v2 should be suspect, got %v", h[1].State)
	}
}

func TestBeliefDiff(t *testing.T) {
	v1 := BeliefVersion{Content: "IIT is weakened", Confidence: 0.70, Frame: "empirical", State: BeliefActive}
	v2 := BeliefVersion{Content: "IIT is substantially weakened", Confidence: 0.85, Frame: "empirical", State: BeliefActive}
	diff := Diff(v1, v2)
	t.Logf("Diff: %s", diff)
	if !strings.Contains(diff, "content:") { t.Error("diff should mention content change") }
	if !strings.Contains(diff, "confidence:") { t.Error("diff should mention confidence change") }
}

func TestVersionAt(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(24 * time.Hour)
	t2 := t0.Add(48 * time.Hour)
	s.RegisterFrame(Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayNone}})

	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "Finding", Timestamp: t0})
	s.Believe(&Belief{ID: "b1", Frame: "empirical", Content: "Original", Confidence: 0.8, AssertedAt: t0, Derivation: []string{"r1"}})

	v := s.versions.VersionAt("b1", t0.Add(-time.Second))
	if v != nil { t.Errorf("expected nil before assertion, got v%d", v.Version) }

	s.Revise("r1", "Corrected", 0, "correction", t1)
	s.ReAssert("b1", "Revised", 0.65, t1)

	vAt1 := s.versions.VersionAt("b1", t1)
	if vAt1 == nil { t.Error("expected version at t1") } else {
		t.Logf("Version at t1: v%d content=%q", vAt1.Version, vAt1.Content)
	}
	vAt2 := s.versions.VersionAt("b1", t2)
	if vAt2 == nil { t.Error("expected version at t2") }
}
