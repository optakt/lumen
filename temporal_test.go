package lumen

import (
	"testing"
	"time"
)

func TestTemporalBasic(t *testing.T) {
	s := NewStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := Frame{Name: "test", Decay: DecayPolicy{Kind: DecayNone}}
	s.RegisterFrame(f)

	// Assert records at known times
	s.Assert(&Record{ID: "r1", Frame: "test", Content: "Record 1", Timestamp: base})
	s.Assert(&Record{ID: "r2", Frame: "test", Content: "Record 2", Timestamp: base.Add(24 * time.Hour)})

	// Assert belief after both records
	s.Believe(&Belief{
		ID: "b1", Frame: "test", Content: "Belief from r1 and r2",
		Confidence: 0.8, AssertedAt: base.Add(48 * time.Hour),
		Derivation: []string{"r1", "r2"},
	})

	// StateAt: what existed before r2?
	stateBeforeR2 := s.Temporal.StateAt(base.Add(12 * time.Hour))
	if len(stateBeforeR2) != 1 || stateBeforeR2[0] != "r1" {
		t.Errorf("expected [r1] before r2, got %v", stateBeforeR2)
	}

	// StateAt: what exists after b1?
	stateAfterAll := s.Temporal.StateAt(base.Add(72 * time.Hour))
	if len(stateAfterAll) != 3 {
		t.Errorf("expected 3 nodes after all assertions, got %d: %v", len(stateAfterAll), stateAfterAll)
	}

	// EarliestSupportableAt: b1 can't exist before r2 (its latest source)
	earliest, err := s.Temporal.EarliestSupportableAt("b1")
	if err != nil {
		t.Fatalf("EarliestSupportableAt failed: %v", err)
	}
	if !earliest.Equal(base.Add(24 * time.Hour)) {
		t.Errorf("expected earliest = r2's time, got %v", earliest)
	}
	t.Logf("b1 earliest supportable: %v (r2 asserted at %v)", earliest, base.Add(24*time.Hour))
}

func TestTemporalCounterfactual(t *testing.T) {
	s := NewStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := Frame{Name: "test", Decay: DecayPolicy{Kind: DecayNone}}
	s.RegisterFrame(f)

	// r1 → b1 → b2 (linear chain)
	// r2 → b3 (independent)
	s.Assert(&Record{ID: "r1", Frame: "test", Content: "Root", Timestamp: base})
	s.Assert(&Record{ID: "r2", Frame: "test", Content: "Independent", Timestamp: base})
	s.Believe(&Belief{ID: "b1", Frame: "test", Content: "From r1", Confidence: 0.9, AssertedAt: base.Add(time.Hour), Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "b2", Frame: "test", Content: "From b1", Confidence: 0.8, AssertedAt: base.Add(2 * time.Hour), Derivation: []string{"b1"}})
	s.Believe(&Belief{ID: "b3", Frame: "test", Content: "From r2", Confidence: 0.7, AssertedAt: base.Add(time.Hour), Derivation: []string{"r2"}})

	// If r1 had never been asserted, b1 and b2 would not exist
	removed := s.Temporal.CounterfactualRemoval("r1")
	t.Logf("Counterfactual removal of r1: %v", removed)

	containsB1, containsB2, containsB3 := false, false, false
	for _, id := range removed {
		switch id {
		case "b1":
			containsB1 = true
		case "b2":
			containsB2 = true
		case "b3":
			containsB3 = true
		}
	}
	if !containsB1 {
		t.Error("b1 should be in counterfactual removal of r1")
	}
	if !containsB2 {
		t.Error("b2 should be in counterfactual removal of r1 (depends on b1)")
	}
	if containsB3 {
		t.Error("b3 should NOT be in counterfactual removal of r1 (depends on r2)")
	}
}

func TestTemporalWouldExistWithout(t *testing.T) {
	s := NewStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := Frame{Name: "test", Decay: DecayPolicy{Kind: DecayNone}}
	s.RegisterFrame(f)

	s.Assert(&Record{ID: "r1", Frame: "test", Content: "Finding", Timestamp: base})
	s.Believe(&Belief{ID: "b1", Frame: "test", Content: "Conclusion", Confidence: 0.8, AssertedAt: base.Add(time.Hour), Derivation: []string{"r1"}})

	if s.Temporal.WouldExistWithout("b1", "r1") {
		t.Error("b1 would NOT exist without r1")
	}
	if !s.Temporal.WouldExistWithout("b1", "some-other-record") {
		t.Error("b1 WOULD exist without some-other-record (no dependency)")
	}
}

func TestTemporalTimeline(t *testing.T) {
	s := NewStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := Frame{Name: "test", Decay: DecayPolicy{Kind: DecayNone}}
	s.RegisterFrame(f)

	s.Assert(&Record{ID: "r2", Frame: "test", Content: "Second", Timestamp: base.Add(2 * time.Hour)})
	s.Assert(&Record{ID: "r1", Frame: "test", Content: "First", Timestamp: base})
	s.Assert(&Record{ID: "r3", Frame: "test", Content: "Third", Timestamp: base.Add(4 * time.Hour)})

	timeline := s.Temporal.Timeline()
	if len(timeline) != 3 {
		t.Fatalf("expected 3 events, got %d", len(timeline))
	}
	// Should be in chronological order
	if timeline[0].NodeID != "r1" || timeline[1].NodeID != "r2" || timeline[2].NodeID != "r3" {
		t.Errorf("timeline not chronologically ordered: %v", func() []string {
			ids := make([]string, len(timeline))
			for i, e := range timeline {
				ids[i] = e.NodeID
			}
			return ids
		}())
	}
	t.Logf("Timeline: r1 → r2 → r3 (chronological)")
}
