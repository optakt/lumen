package lumen

import (
	"strings"
	"testing"
	"time"
)

// setupStalenessStore creates a store where b1 derives from b-parent,
// and b-parent has decayed significantly over a long time window.
func setupStalenessStore(t *testing.T) (*Store, time.Time) {
	t.Helper()
	s := NewStore()
	// Frame with fast decay (1 day halflife) and mark_suspect policy.
	s.RegisterFrame(Frame{
		Name:              "fast-decay",
		Composition:       CompositionBayesian,
		Decay:             DecayPolicy{Kind: DecayExponential, Halflife: 24 * time.Hour},
		OnStaleDerivation: StaleMarkSuspect,
	})
	// Frame with no decay for the parent belief.
	s.RegisterFrame(Frame{
		Name:        "no-decay",
		Composition: CompositionBayesian,
		Decay:       DecayPolicy{Kind: DecayExponential, Halflife: 24 * time.Hour},
	})

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := s.Assert(&Record{ID: "r1", Content: "Base record.", Frame: "no-decay", Timestamp: t0}); err != nil {
		t.Fatal(err)
	}
	if err := s.Believe(&Belief{
		ID:         "b-parent",
		Content:    "Parent belief (will decay fast).",
		Confidence: 0.90,
		Frame:      "no-decay",
		AssertedAt: t0,
		Derivation: []string{"r1"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Believe(&Belief{
		ID:         "b1",
		Content:    "Belief that depends on b-parent.",
		Confidence: 0.80,
		Frame:      "fast-decay",
		AssertedAt: t0,
		Derivation: []string{"b-parent"},
	}); err != nil {
		t.Fatal(err)
	}

	return s, t0
}

func TestStaleDerivers(t *testing.T) {
	s, t0 := setupStalenessStore(t)

	// At t0: b-parent has decayed 0 halflives → 0.90. Not stale.
	stale := s.StaleDerivers("b1", t0)
	if len(stale) != 0 {
		t.Errorf("expected no stale derivers at t0, got %v", stale)
	}

	// At t0 + 10 days: b-parent at 10 halflives → 0.90 * 2^-10 ≈ 0.00088. Stale.
	t10 := t0.Add(10 * 24 * time.Hour)
	stale10 := s.StaleDerivers("b1", t10)
	if len(stale10) == 0 {
		t.Error("expected b-parent to be stale after 10 halflives")
	}
	t.Logf("stale derivers after 10 days: %v", stale10)
}

func TestOnStaleDeriversMarkSuspect(t *testing.T) {
	s, t0 := setupStalenessStore(t)

	// After 10 days, Query() should apply mark_suspect policy.
	t10 := t0.Add(10 * 24 * time.Hour)
	result, err := s.Query("b1", t10)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// b1 should now be suspect due to stale derivation.
	if result.State != BeliefSuspect {
		t.Errorf("expected BeliefSuspect after stale derivation, got %v", result.State)
	}
	t.Logf("b1 state after 10 days: %v (correct: suspect)", result.State)
}

func TestOnStaleDeriversFreshIsActive(t *testing.T) {
	s, t0 := setupStalenessStore(t)

	// At t0, no staleness → b1 remains active.
	result, err := s.Query("b1", t0)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.State != BeliefActive {
		t.Errorf("expected BeliefActive at t0, got %v", result.State)
	}
}

func TestOnStaleDeriversFailPolicy(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{
		Name:              "strict",
		Composition:       CompositionBayesian,
		Decay:             DecayPolicy{Kind: DecayExponential, Halflife: 24 * time.Hour},
		OnStaleDerivation: StaleFail,
	})
	s.RegisterFrame(Frame{
		Name:  "base",
		Decay: DecayPolicy{Kind: DecayExponential, Halflife: 24 * time.Hour},
	})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = s.Assert(&Record{ID: "r1", Content: "R.", Frame: "base", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "b-src", Content: "Source.", Confidence: 0.90, Frame: "base", AssertedAt: t0, Derivation: []string{"r1"}})
	_ = s.Believe(&Belief{ID: "b-strict", Content: "Strict.", Confidence: 0.80, Frame: "strict", AssertedAt: t0, Derivation: []string{"b-src"}})

	// After 10 days the source belief is stale → fail policy → error from Query.
	t10 := t0.Add(10 * 24 * time.Hour)
	_, err := s.Query("b-strict", t10)
	if err == nil {
		t.Error("expected error from fail policy, got nil")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Errorf("error should mention stale: %v", err)
	}
	t.Logf("fail policy error: %v", err)
}

func TestStalenessMentionedInExplain(t *testing.T) {
	s, t0 := setupStalenessStore(t)
	t10 := t0.Add(10 * 24 * time.Hour)

	explanation, err := s.Explain("b1", t10)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if !strings.Contains(explanation, "Stale") {
		n := 200; if len(explanation) < n { n = len(explanation) }
		t.Errorf("Explain should mention stale source beliefs:\n%s", explanation[:n])
	}
	t.Logf("stale mention in Explain: found")
}


func TestOnStaleDeriversRetryMarksSuspect(t *testing.T) {
	// on_stale_derivation: retry should both return an error AND mark the belief suspect.
	// This prevents repeated errors on subsequent queries once it's known stale.
	s := NewStore()
	s.RegisterFrame(Frame{
		Name:              "retry-frame",
		Composition:       CompositionBayesian,
		Decay:             DecayPolicy{Kind: DecayExponential, Halflife: 24 * time.Hour},
		OnStaleDerivation: StaleRetry,
	})
	s.RegisterFrame(Frame{
		Name:  "source-frame",
		Decay: DecayPolicy{Kind: DecayExponential, Halflife: 24 * time.Hour},
	})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = s.Assert(&Record{ID: "r1", Content: "R.", Frame: "source-frame", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "src", Content: "Source.", Confidence: 0.90, Frame: "source-frame", AssertedAt: t0, Derivation: []string{"r1"}})
	_ = s.Believe(&Belief{ID: "b-retry", Content: "Retry.", Confidence: 0.80, Frame: "retry-frame", AssertedAt: t0, Derivation: []string{"src"}})

	// After 10 halflives, src is stale.
	t10 := t0.Add(10 * 24 * time.Hour)
	_, err := s.Query("b-retry", t10)

	// Should return error.
	if err == nil {
		t.Error("expected error from retry policy")
	}

	// AND belief should now be BeliefSuspect (not still Active).
	s.mu.RLock()
	b := s.beliefs["b-retry"]
	s.mu.RUnlock()
	if b.State != BeliefSuspect {
		t.Errorf("retry policy should mark belief suspect; got state %v", b.State)
	}
	t.Logf("retry policy: error=%v, state=%v (correct)", err, b.State)

	// Subsequent query should show BeliefSuspect state (not error again).
	// On second query, StaleDerivers still finds stale sources, but applyOnStaleDerivation
	// sees b is already BeliefSuspect, so mark_suspect is a no-op. But retry still
	// returns an error because the sources are still stale. This is correct.
	_, err2 := s.Query("b-retry", t10)
	if err2 == nil {
		t.Error("retry should still error on second query (sources still stale)")
	}
	t.Logf("second query error: %v (expected)", err2)
}
