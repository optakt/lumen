package lumen

import (
	"testing"
	"time"
)

func TestImpactScan(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: "none"}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// r1 → b1 → b2.  r2 → b3 (unrelated).
	_ = s.Assert(&Record{ID: "r1", Frame: "f", Content: "Root.", Timestamp: t0})
	_ = s.Assert(&Record{ID: "r2", Frame: "f", Content: "Other.", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "b1", Frame: "f", Content: "Derived.", Confidence: 0.80, AssertedAt: t0, Derivation: []string{"r1"}})
	_ = s.Believe(&Belief{ID: "b2", Frame: "f", Content: "Transitive.", Confidence: 0.70, AssertedAt: t0, Derivation: []string{"b1"}})
	_ = s.Believe(&Belief{ID: "b3", Frame: "f", Content: "Unrelated.", Confidence: 0.65, AssertedAt: t0, Derivation: []string{"r2"}})

	entries, err := s.ImpactScan("r1", t0)
	if err != nil {
		t.Fatalf("ImpactScan: %v", err)
	}

	t.Log("Impact of retracting r1:")
	for _, e := range entries {
		t.Logf("  %s", e)
	}

	// b1 should be affected (direct, hop 1) and b2 (transitive, hop 2).
	foundB1, foundB2, foundB3 := false, false, false
	for _, e := range entries {
		switch e.BeliefID {
		case "b1": foundB1 = true
			if !e.DirectlyLinked { t.Error("b1 should be directly linked") }
			if e.EstimatedConf != 0 { t.Errorf("b1 (sole source retracted) should drop to 0; got %.3f", e.EstimatedConf) }
		case "b2": foundB2 = true
			if e.Distance != 2 { t.Errorf("b2 should be at distance 2, got %d", e.Distance) }
		case "b3": foundB3 = true
		}
	}
	if !foundB1 { t.Error("b1 should appear in impact scan") }
	if !foundB2 { t.Error("b2 should appear in impact scan (transitive)") }
	if foundB3  { t.Error("b3 should NOT appear in impact scan (unrelated source)") }
}

func TestImpactScanUnknownSource(t *testing.T) {
	s := NewStore()
	_, err := s.ImpactScan("nonexistent", time.Now())
	if err == nil {
		t.Error("expected error for unknown source")
	}
}

func TestImpactScanBeliefSource(t *testing.T) {
	// Impact of retracting a belief (not a record) as source.
	// b1 derives from r1; b2 derives from b1; b3 derives from b2.
	// What happens when we "retract" b1 (ask for its impact on dependents)?
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: "none"}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Assert(&Record{ID: "r1", Frame: "f", Content: "Root.", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "b1", Frame: "f", Content: "Layer 1.", Confidence: 0.90, AssertedAt: t0, Derivation: []string{"r1"}})
	_ = s.Believe(&Belief{ID: "b2", Frame: "f", Content: "Layer 2.", Confidence: 0.80, AssertedAt: t0, Derivation: []string{"b1"}})
	_ = s.Believe(&Belief{ID: "b3", Frame: "f", Content: "Layer 3.", Confidence: 0.70, AssertedAt: t0, Derivation: []string{"b2"}})

	entries, err := s.ImpactScan("b1", t0)
	if err != nil { t.Fatalf("ImpactScan: %v", err) }

	t.Logf("Impact of losing belief b1:")
	for _, e := range entries { t.Logf("  %s", e) }

	// b2 directly derives from b1 → should appear at hop 1.
	// b3 transitively derives through b2 → hop 2.
	var hops []int
	for _, e := range entries { hops = append(hops, e.Distance) }
	hasHop1, hasHop2 := false, false
	for _, h := range hops {
		if h == 1 { hasHop1 = true }
		if h == 2 { hasHop2 = true }
	}
	if !hasHop1 { t.Error("should have a hop-1 entry (b2)") }
	if !hasHop2 { t.Error("should have a hop-2 entry (b3)") }

	// b2's sole source (b1) is removed → b2 should drop to 0.
	for _, e := range entries {
		if e.BeliefID == "b2" && e.EstimatedConf > 0.001 {
			t.Errorf("b2 (sole source removed) should drop to ~0; got %.3f", e.EstimatedConf)
		}
	}
}

func TestImpactScanPartialConfidencePropagate(t *testing.T) {
	// Verify that transitive impact uses the estimated (not zero) confidence.
	// r1 ─→ b1 (0.90, sole source r1) ─→ b2 (0.80, two sources: b1 at 0.90, r2 at 1.0)
	// Impact of retracting r1:
	//   b1 (hop 1): drops to 0 (sole source retracted)
	//   b2 (hop 2): b1 goes to 0, but r2 is still 1.0.
	//               noisy-or of [b1=0, r2=1.0] vs [b1=0.90, r2=1.0]:
	//               original nor = 1 - (1-0.9)*(1-1.0) = 1 - 0 = 1.0
	//               (both sources near certain, so b2 is highly supported)
	//               after cascade, nor([b1=0, r2=1]) = 1 - (1-0)*(1-1.0) = 1 - 0 = 1.0
	//               So b2 stays at 0.80 (r2 alone fully supports it).
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: "none"}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Assert(&Record{ID: "r1", Frame: "f", Content: "R1.", Timestamp: t0})
	_ = s.Assert(&Record{ID: "r2", Frame: "f", Content: "R2.", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "b1", Frame: "f", Content: "Layer 1.", Confidence: 0.90, AssertedAt: t0, Derivation: []string{"r1"}})
	_ = s.Believe(&Belief{ID: "b2", Frame: "f", Content: "Layer 2.", Confidence: 0.80, AssertedAt: t0, Derivation: []string{"b1", "r2"}})

	entries, err := s.ImpactScan("r1", t0)
	if err != nil { t.Fatalf("ImpactScan: %v", err) }

	t.Logf("Impact of retracting r1 (b2 has second source r2 at 1.0):")
	for _, e := range entries { t.Logf("  %s", e) }

	for _, e := range entries {
		switch e.BeliefID {
		case "b1":
			if e.EstimatedConf > 0.001 { t.Errorf("b1 sole source lost; should be 0, got %.3f", e.EstimatedConf) }
		case "b2":
			// b2's other source r2 is at full confidence, so b2 should be stable.
			if e.EstimatedConf < 0.75 {
				t.Errorf("b2 has r2 at 1.0 confidence; should stay ~0.80, got %.3f", e.EstimatedConf)
			}
		}
	}
}
