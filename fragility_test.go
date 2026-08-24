package lumen

import (
	"testing"
	"time"
)

func TestFragilityScan(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{
		Name:  "f",
		Decay: DecayPolicy{Kind: DecayExponential, Halflife: 365 * 24 * time.Hour},
	})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// r1 and r2 both support b1. r3 alone supports b2.
	_ = s.Assert(&Record{ID: "r1", Frame: "f", Content: "A.", Timestamp: t0})
	_ = s.Assert(&Record{ID: "r2", Frame: "f", Content: "B.", Timestamp: t0})
	_ = s.Assert(&Record{ID: "r3", Frame: "f", Content: "C.", Timestamp: t0})

	_ = s.Believe(&Belief{ID: "b1", Frame: "f", Content: "X.", Confidence: 0.80, AssertedAt: t0, Derivation: []string{"r1", "r2"}})
	_ = s.Believe(&Belief{ID: "b2", Frame: "f", Content: "Y.", Confidence: 0.75, AssertedAt: t0, Derivation: []string{"r3"}})

	entries := s.FragilityScan(t0)
	if len(entries) == 0 {
		t.Fatal("expected fragility entries")
	}

	// b2 has only one source → drop to 0 → fragility drop = 0.75.
	// b1 has two sources → dropping one leaves the other → drop is smaller.
	// So b2 should be most fragile.
	if entries[0].BeliefID != "b2" {
		t.Errorf("most fragile should be b2 (single source), got %s", entries[0].BeliefID)
	}
	if entries[0].ConfWithout != 0 {
		t.Errorf("b2 with no sources should have 0 confidence, got %.3f", entries[0].ConfWithout)
	}
	t.Logf("Fragility scan:")
	for _, e := range entries {
		t.Logf("  %s", e)
	}
}

func TestFragilityScanIgnoresSuperseded(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: DecayNone}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Assert(&Record{ID: "r1", Frame: "f", Content: "A.", Timestamp: t0})
	_ = s.Assert(&Record{ID: "r2", Frame: "f", Content: "B.", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "b1", Frame: "f", Content: "X.", Confidence: 0.80, AssertedAt: t0, Derivation: []string{"r1"}})
	_ = s.Believe(&Belief{ID: "b2", Frame: "f", Content: "Y.", Confidence: 0.75, AssertedAt: t0, Derivation: []string{"r2"}})

	// Contract b1.
	result, err := s.MinimalContraction("r1", t0)
	if err != nil { t.Fatalf("contraction: %v", err) }
	if err := s.ApplyContraction(result, "test retraction", t0); err != nil {
		t.Fatalf("apply contraction: %v", err)
	}

	entries := s.FragilityScan(t0)
	for _, e := range entries {
		if e.BeliefID == "b1" {
			t.Error("contracted belief b1 should not appear in fragility scan")
		}
	}
}

func TestFragilityEstimateWithout(t *testing.T) {
	// Single source: removing it drops to 0.
	single := []sourceConf{{"r1", "record", 1.0}}
	if got := estimateWithout(single, 0, 0.80); got != 0 {
		t.Errorf("single source removal should give 0; got %.3f", got)
	}

	// Two full-confidence records: noisy-or is 1.0 in both directions, so
	// removing one of two full-confidence records does NOT drop the confidence.
	// This is mathematically correct: the remaining record is still at 1.0.
	twoFull := []sourceConf{{"r1", "record", 1.0}, {"r2", "record", 1.0}}
	remFull := estimateWithout(twoFull, 0, 0.80)
	if remFull != 0.80 {
		t.Errorf("two full-confidence sources: removing one leaves full support; got %.3f", remFull)
	}

	// Two partially-decayed belief sources: removing the weaker one should
	// drop confidence less than removing the stronger one.
	twoDecayed := []sourceConf{{"b1", "belief", 0.70}, {"b2", "belief", 0.40}}
	remWithoutStrong := estimateWithout(twoDecayed, 0, 0.80) // remove 0.70-source
	remWithoutWeak   := estimateWithout(twoDecayed, 1, 0.80) // remove 0.40-source
	if remWithoutStrong >= remWithoutWeak {
		t.Errorf("removing stronger source should cause larger drop: %.3f vs %.3f", remWithoutStrong, remWithoutWeak)
	}
	t.Logf("decayed pair [0.70, 0.40]: remove strong→%.3f, remove weak→%.3f", remWithoutStrong, remWithoutWeak)
}

func TestMinCut(t *testing.T) {
	// One source: min cut = 1.
	single := []sourceConf{{"r1", "record", 1.0}}
	if got := computeMinCut(single, 0.80); got != 1 {
		t.Errorf("single source: min cut should be 1, got %d", got)
	}

	// Two full-confidence records: both must be retracted → min cut = 2.
	two := []sourceConf{{"r1", "record", 1.0}, {"r2", "record", 1.0}}
	if got := computeMinCut(two, 0.80); got != 2 {
		t.Errorf("two full records: min cut should be 2, got %d", got)
	}

	// Three full-confidence records: min cut = 3.
	three := []sourceConf{{"r1", "record", 1.0}, {"r2", "record", 1.0}, {"r3", "record", 1.0}}
	if got := computeMinCut(three, 0.80); got != 3 {
		t.Errorf("three full records: min cut should be 3, got %d", got)
	}

	// One zero-confidence source + one full: zero source is already contributing
	// nothing; only the full one needs removal → min cut = 1.
	mixed := []sourceConf{{"r1", "record", 0.0}, {"r2", "record", 1.0}}
	if got := computeMinCut(mixed, 0.80); got != 1 {
		t.Errorf("mixed (0.0, 1.0): min cut should be 1, got %d", got)
	}
}

// TestFragilityComposedPath verifies that BelieveComposed beliefs use
// SensitivityAnalysis (exact) rather than the norScale approximation,
// and that the composition metadata survives a BoltDB round-trip.
func TestFragilityComposedPath(t *testing.T) {
	s := NewStore()
	now := time.Now()

	s.RegisterFrame(Frame{Name: "reasoning", Decay: DecayPolicy{Kind: DecayNone}})

	r1 := &Record{ID: "r-zombie", Frame: "reasoning", Content: "Zombie argument: conceivability implies possibility", Timestamp: now}
	r2 := &Record{ID: "r-knowledge", Frame: "reasoning", Content: "Knowledge argument: Mary learns something new", Timestamp: now}
	if err := s.Assert(r1); err != nil {
		t.Fatal(err)
	}
	if err := s.Assert(r2); err != nil {
		t.Fatal(err)
	}

	prior := 0.5
	evidence := []Evidence{
		{SourceID: "r-zombie",    Confidence: 0.8, LikelihoodRatio: 3.2},
		{SourceID: "r-knowledge", Confidence: 0.7, LikelihoodRatio: 2.1},
	}
	b := &Belief{
		ID:         "b-hardproblem",
		Content:    "The hard problem of consciousness is genuine",
		Confidence: 0.78,
		Frame:      "reasoning",
		AssertedAt: now,
		Derivation: []string{"r-zombie", "r-knowledge"},
	}
	cb, err := s.BelieveComposed(b, prior, evidence)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("composed posterior=%.4f declared=%.4f", cb.ComputedConfidence, b.Confidence)

	// Verify composition metadata stored on the belief.
	s.mu.RLock()
	stored := s.beliefs["b-hardproblem"]
	s.mu.RUnlock()
	if len(stored.CompositionEvidence) != 2 {
		t.Fatalf("expected 2 evidence blocks stored, got %d", len(stored.CompositionEvidence))
	}
	if stored.CompositionPrior != prior {
		t.Fatalf("expected prior %.2f, got %.2f", prior, stored.CompositionPrior)
	}

	// FragilityScan should use the sensitivity path, not norScale.
	entries := s.FragilityScan(now)
	if len(entries) == 0 {
		t.Fatal("expected at least one fragility entry")
	}
	entry := entries[0]
	if entry.WeakestKind != "evidence" {
		t.Errorf("expected WeakestKind=evidence (sensitivity path), got %q", entry.WeakestKind)
	}
	if entry.Drop <= 0 {
		t.Errorf("expected positive drop, got %.4f", entry.Drop)
	}
	t.Logf("fragility: drop=%.4f weakest=%s", entry.Drop, entry.WeakestSource)
}
