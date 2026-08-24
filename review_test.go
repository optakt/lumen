package lumen

import (
	"testing"
	"time"
)

// TestBelieveComposedRetractCascade proves that beliefs created via
// BelieveComposed participate in the retraction cascade. Regression test
// for review finding: BelieveComposed updated s.dependents but never added
// Graph edges, and markSuspect reads only the Graph.
func TestBelieveComposedRetractCascade(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: DecayNone}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Assert(&Record{ID: "r1", Frame: "f", Content: "Evidence.", Timestamp: t0})

	ev := []Evidence{{SourceID: "e1", LikelihoodRatio: 4.0, Confidence: 0.9}}
	_, err := s.BelieveComposed(&Belief{
		ID: "b1", Frame: "f", Content: "Composed.", Confidence: 0.67,
		AssertedAt: t0, Derivation: []string{"r1"},
	}, 0.30, ev)
	if err != nil {
		t.Fatalf("BelieveComposed: %v", err)
	}

	if err := s.Retract("r1", "review test", t0.Add(time.Hour)); err != nil {
		t.Fatalf("Retract: %v", err)
	}

	q, err := s.Query("b1", t0.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if q.State != BeliefSuspect {
		t.Errorf("belief created via BelieveComposed should be suspect after source retraction; state=%v", q.State)
	}
}

// TestAssertRejectsBeliefIDCollision proves the ID namespace is enforced
// symmetrically: Believe rejects record IDs, Assert must reject belief IDs.
func TestAssertRejectsBeliefIDCollision(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: DecayNone}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Believe(&Belief{ID: "x", Frame: "f", Content: "B.", Confidence: 0.5, AssertedAt: t0})
	if err := s.Assert(&Record{ID: "x", Frame: "f", Content: "R.", Timestamp: t0}); err == nil {
		t.Error("Assert should reject an ID already used by a belief")
	}
}

// TestReAssertRefreshesCrossFrameSnapshot: re-asserting a belief must refresh
// cross-frame snapshots. Otherwise the min() clamp in CurrentConfidence keeps
// decaying from the ORIGINAL import time, dragging a freshly re-asserted
// belief toward zero — the retrodiction problem re-entering via ReAssert.
func TestReAssertRefreshesCrossFrameSnapshot(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "stable", Decay: DecayPolicy{Kind: DecayNone}})
	s.RegisterFrame(Frame{Name: "fast", Decay: DecayPolicy{Kind: DecayExponential, Halflife: 7 * 24 * time.Hour}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Assert(&Record{ID: "r1", Frame: "stable", Content: "Root.", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "src", Frame: "stable", Content: "Stable source.", Confidence: 0.90, AssertedAt: t0, Derivation: []string{"r1"}})
	// Cross-frame import: belief in "fast" frame derives from belief in "stable" frame.
	_ = s.Believe(&Belief{ID: "b1", Frame: "fast", Content: "Imported.", Confidence: 0.85, AssertedAt: t0, Derivation: []string{"src"}})

	// 70 days later (10 halflives of the receiving frame), re-assert with fresh confidence.
	t70 := t0.Add(70 * 24 * time.Hour)
	if err := s.ReAssert("b1", "Imported, re-evaluated.", 0.85, t70); err != nil {
		t.Fatalf("ReAssert: %v", err)
	}

	// Immediately after re-assertion the belief should report ~0.85.
	// Without snapshot refresh, the cross-frame floor is 0.90 * 2^-10 ≈ 0.0009.
	q, err := s.Query("b1", t70)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if q.CurrentConfidence < 0.80 {
		t.Errorf("freshly re-asserted belief dragged down by stale cross-frame snapshot: %.4f (want ~0.85)", q.CurrentConfidence)
	}
}

// TestContractionDeadSourceNotClean: a source that is already retracted (or a
// previously contracted belief) must not count as a "clean path" during
// MinimalContraction. Otherwise a belief whose only other support is already
// dead survives contraction with no live evidence.
func TestContractionDeadSourceNotClean(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: DecayNone}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Assert(&Record{ID: "r1", Frame: "f", Content: "Live evidence.", Timestamp: t0})
	_ = s.Assert(&Record{ID: "r2", Frame: "f", Content: "Old evidence.", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "b1", Frame: "f", Content: "Doubly supported.", Confidence: 0.8, AssertedAt: t0, Derivation: []string{"r1", "r2"}})

	// r2 is retracted first — b1 becomes suspect but keeps r1 as live support.
	if err := s.Retract("r2", "earlier retraction", t0.Add(time.Hour)); err != nil {
		t.Fatalf("Retract r2: %v", err)
	}

	// Now contract r1. b1's only remaining source, r2, is already dead.
	result, err := s.MinimalContraction("r1", t0.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("MinimalContraction: %v", err)
	}

	removed := false
	for _, id := range result.Removed {
		if id == "b1" { removed = true }
	}
	if !removed {
		t.Errorf("b1 has no live support after contracting r1 (r2 already retracted); must be removed. Removed=%v Preserved=%v",
			result.Removed, result.Preserved)
	}
}

// TestContractionMergedSourceStillClean: beliefs retired by merge (Superseded,
// empty ContractedBy) remain valid support during contraction — they were
// folded into a merged belief, not epistemically removed.
func TestContractionMergedSourceStillClean(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: DecayNone}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Assert(&Record{ID: "r1", Frame: "f", Content: "A.", Timestamp: t0})
	_ = s.Assert(&Record{ID: "r2", Frame: "f", Content: "B.", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "bA", Frame: "f", Content: "From A.", Confidence: 0.7, AssertedAt: t0, Derivation: []string{"r1"}})
	_ = s.Believe(&Belief{ID: "bB", Frame: "f", Content: "From B.", Confidence: 0.65, AssertedAt: t0, Derivation: []string{"r2"}})
	if _, err := s.MergeBeliefs("bA", "bB", "m", "Merged.", "", "noisy-or", true, t0.Add(time.Hour)); err != nil {
		t.Fatalf("MergeBeliefs: %v", err)
	}

	// Contract r1: m derives from bA (contaminated via r1) and bB (merged-retired,
	// clean via r2). bB must count as live support, so m is preserved.
	result, err := s.MinimalContraction("r1", t0.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("MinimalContraction: %v", err)
	}
	for _, id := range result.Removed {
		if id == "m" {
			t.Errorf("merged belief m should be preserved (bB is merged-retired but still valid support); Removed=%v", result.Removed)
		}
	}
}
