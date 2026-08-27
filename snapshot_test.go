package lumen_test

import (
	"testing"
	"time"

	lumen "github.com/optakt/lumen"
)

func TestSnapshotAtBasic(t *testing.T) {
	t.Parallel()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1  := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t2  := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	s := lumen.NewStore()
	s.RegisterFrame(lumen.Frame{Name: "f", Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}})

	// Record asserted in 2020.
	r1 := &lumen.Record{ID: "r1", Frame: "f", Content: "early fact", Timestamp: t1}
	if err := s.Assert(r1); err != nil {
		t.Fatalf("Assert r1: %v", err)
	}

	// Record asserted in 2023.
	r2 := &lumen.Record{ID: "r2", Frame: "f", Content: "later fact", Timestamp: t2}
	if err := s.Assert(r2); err != nil {
		t.Fatalf("Assert r2: %v", err)
	}

	// Belief deriving from r1 only (exists from 2020).
	b1 := &lumen.Belief{
		ID: "b1", Frame: "f", Content: "early belief",
		Confidence: 0.80, AssertedAt: t1,
		Derivation: []string{"r1"},
	}
	if err := s.Believe(b1); err != nil {
		t.Fatalf("Believe b1: %v", err)
	}

	// Belief deriving from r2 (exists from 2023).
	b2 := &lumen.Belief{
		ID: "b2", Frame: "f", Content: "later belief",
		Confidence: 0.60, AssertedAt: t2,
		Derivation: []string{"r1", "r2"},
	}
	if err := s.Believe(b2); err != nil {
		t.Fatalf("Believe b2: %v", err)
	}

	// Snapshot at 2021 — only r1 and b1 should be visible.
	mid := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	snap := s.SnapshotAt(mid)

	// b1 must be queryable in the snapshot.
	qb1, err := snap.Query("b1", now)
	if err != nil {
		t.Fatalf("Query b1 in snapshot: %v", err)
	}
	if qb1.CurrentConfidence < 0.79 {
		t.Errorf("b1 confidence = %.2f, want ~0.80", qb1.CurrentConfidence)
	}

	// b2 must NOT be in the snapshot.
	if _, err := snap.Query("b2", now); err == nil {
		t.Error("b2 should not be in 2021 snapshot")
	}

	// r2 must NOT be in the snapshot (it was asserted in 2023).
	all := snap.AllBeliefs(now)
	for _, qr := range all {
		if qr.BeliefID == "b2" {
			t.Error("b2 found in AllBeliefs of 2021 snapshot")
		}
	}

	// Full store still has both beliefs.
	full := s.AllBeliefs(now)
	if len(full) != 2 {
		t.Errorf("full store has %d beliefs, want 2", len(full))
	}
}

func TestSnapshotAtExcludesDependentBelief(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC)

	s := lumen.NewStore()
	s.RegisterFrame(lumen.Frame{Name: "empirical", Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}})

	// Three records, each asserted at a later time.
	for _, rec := range []struct {
		id string
		ts time.Time
	}{
		{"base", t1},
		{"mid", t2},
		{"late", t3},
	} {
		if err := s.Assert(&lumen.Record{ID: rec.id, Frame: "empirical", Content: rec.id + " content", Timestamp: rec.ts}); err != nil {
			t.Fatalf("Assert %s: %v", rec.id, err)
		}
	}

	// Belief that requires all three records — only viable from t3 onwards.
	bAll := &lumen.Belief{
		ID: "b-all", Frame: "empirical", Content: "needs all three",
		Confidence: 0.75, AssertedAt: t3,
		Derivation: []string{"base", "mid", "late"},
	}
	if err := s.Believe(bAll); err != nil {
		t.Fatalf("Believe b-all: %v", err)
	}

	// Snapshot between t2 and t3 — "late" doesn't exist yet, so b-all must be absent.
	snap := s.SnapshotAt(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
	if _, err := snap.Query("b-all", time.Now()); err == nil {
		t.Error("b-all should not appear in snapshot before its last source was asserted")
	}

	// Snapshot at exactly t3 — b-all should now be visible.
	snap2 := s.SnapshotAt(t3)
	if _, err := snap2.Query("b-all", time.Now()); err != nil {
		t.Errorf("b-all should be visible at exactly t3: %v", err)
	}
}

func TestSnapshotAtFramesCarriedOver(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)

	s := lumen.NewStore()
	s.RegisterFrame(lumen.Frame{
		Name:  "philosophical",
		Decay: lumen.DecayPolicy{Kind: lumen.DecayNone},
	})
	s.RegisterFrame(lumen.Frame{
		Name:  "empirical",
		Decay: lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 5 * 365 * 24 * time.Hour},
	})

	r := &lumen.Record{ID: "r", Frame: "philosophical", Content: "axiom", Timestamp: t1}
	if err := s.Assert(r); err != nil {
		t.Fatalf("Assert: %v", err)
	}
	b := &lumen.Belief{
		ID: "b", Frame: "philosophical", Content: "derived belief",
		Confidence: 0.90, AssertedAt: t1,
		Derivation: []string{"r"},
	}
	if err := s.Believe(b); err != nil {
		t.Fatalf("Believe: %v", err)
	}

	snap := s.SnapshotAt(t1)
	qr, err := snap.Query("b", time.Now())
	if err != nil {
		t.Fatalf("Query in snapshot: %v", err)
	}
	// philosophical frame has no decay, so confidence should be unchanged.
	if qr.CurrentConfidence < 0.89 {
		t.Errorf("confidence in philosophical frame should not decay: got %.4f", qr.CurrentConfidence)
	}
	if qr.Frame != "philosophical" {
		t.Errorf("frame = %q, want philosophical", qr.Frame)
	}
}

func TestSnapshotAtOriginalUnchanged(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	s := lumen.NewStore()
	s.RegisterFrame(lumen.Frame{Name: "f", Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}})
	s.Assert(&lumen.Record{ID: "r1", Frame: "f", Content: "c1", Timestamp: t1})  //nolint:errcheck
	s.Assert(&lumen.Record{ID: "r2", Frame: "f", Content: "c2", Timestamp: t2})  //nolint:errcheck
	s.Believe(&lumen.Belief{ID: "b1", Frame: "f", Content: "early", Confidence: 0.7, AssertedAt: t1, Derivation: []string{"r1"}}) //nolint:errcheck
	s.Believe(&lumen.Belief{ID: "b2", Frame: "f", Content: "late", Confidence: 0.5, AssertedAt: t2, Derivation: []string{"r2"}})  //nolint:errcheck

	now := time.Now()
	beforeCount := len(s.AllBeliefs(now))

	// Take an early snapshot.
	_ = s.SnapshotAt(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))

	// Original store must be unchanged.
	afterCount := len(s.AllBeliefs(now))
	if afterCount != beforeCount {
		t.Errorf("original store mutated: had %d beliefs, now %d", beforeCount, afterCount)
	}
}
