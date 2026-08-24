package lumen

import (
	"testing"
)

func TestBeliefHealthBasic(t *testing.T) {
	s, now := setupProvenanceStore(t)

	hs, err := s.BeliefHealth("b3", now)
	if err != nil { t.Fatalf("BeliefHealth: %v", err) }

	t.Logf("\n%s", hs.Render("b3"))

	if hs.Score < 50 {
		t.Errorf("healthy belief should score >50, got %.0f", hs.Score)
	}
	if hs.Grade == "F" {
		t.Errorf("healthy belief should not grade F")
	}
}

func TestBeliefHealthSuspect(t *testing.T) {
	s, now := setupProvenanceStore(t)
	s.Retract("r1", "retracted", now)

	hs, err := s.BeliefHealth("b3", now)
	if err != nil { t.Fatalf("BeliefHealth: %v", err) }

	t.Logf("\n%s", hs.Render("b3 (with retracted source)"))

	if len(hs.Warnings) == 0 {
		t.Error("belief with retracted source should have warnings")
	}
	// Score should be lower than for healthy belief
	hsHealthy, _ := s.BeliefHealth("b4", now) // b4 has no retracted sources
	if hs.Score >= hsHealthy.Score {
		t.Errorf("belief with retracted source (%.0f) should score less than healthy belief (%.0f)",
			hs.Score, hsHealthy.Score)
	}
}

func TestStoreHealth(t *testing.T) {
	s, now := setupProvenanceStore(t)

	hs := s.StoreHealth(now)
	t.Logf("\n%s", hs.Render("full store"))

	if hs.Score == 0 {
		t.Error("non-empty store should have score > 0")
	}

	// After retraction, score should drop
	s.Retract("r1", "test", now)
	hs2 := s.StoreHealth(now)
	t.Logf("After retraction: score=%.0f grade=%s", hs2.Score, hs2.Grade)
	if hs2.Score > hs.Score {
		t.Errorf("store health should decrease after retraction (%.0f > %.0f)", hs2.Score, hs.Score)
	}
}

func TestStoreHealthSummary(t *testing.T) {
	s, now := setupProvenanceStore(t)
	entries := s.StoreHealthSummary(now)

	t.Logf("Health summary (%d beliefs, worst first):", len(entries))
	for _, e := range entries {
		t.Logf("  [%s] %.0f  %s", e.Grade, e.Score, truncate(e.Content, 50))
	}

	if len(entries) != len(s.AllBeliefs(now)) {
		t.Errorf("summary should include all beliefs")
	}
}
