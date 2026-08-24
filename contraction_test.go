package lumen

import (
	"testing"
	"time"
)

func setupContractionStore(t *testing.T) (*Store, time.Time) {
	t.Helper()
	s := NewStore()
	now := time.Now()
	f := Frame{Name: "test", Decay: DecayPolicy{Kind: "none"}}
	s.RegisterFrame(f)

	// Record graph:
	//   r1 (empirical finding)
	//   r2 (independent finding)
	//
	// Belief graph:
	//   b1 derives from r1 only
	//   b2 derives from r2 only
	//   b3 derives from r1 AND r2 (has a clean path via r2)
	//   b4 derives from b1 only (transitively depends on r1)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup error: %v", err)
		}
	}

	must(s.Assert(&Record{ID: "r1", Frame: "test", Content: "Empirical finding A", Timestamp: now}))
	must(s.Assert(&Record{ID: "r2", Frame: "test", Content: "Independent finding B", Timestamp: now}))
	must(s.Believe(&Belief{ID: "b1", Frame: "test", Content: "Conclusion from A only", Confidence: 0.8, AssertedAt: now, Derivation: []string{"r1"}}))
	must(s.Believe(&Belief{ID: "b2", Frame: "test", Content: "Conclusion from B only", Confidence: 0.7, AssertedAt: now, Derivation: []string{"r2"}}))
	must(s.Believe(&Belief{ID: "b3", Frame: "test", Content: "Conclusion from A and B", Confidence: 0.9, AssertedAt: now, Derivation: []string{"r1", "r2"}}))
	must(s.Believe(&Belief{ID: "b4", Frame: "test", Content: "Downstream of b1", Confidence: 0.6, AssertedAt: now, Derivation: []string{"b1"}}))

	return s, now
}

func TestMinimalContractionBasic(t *testing.T) {
	s, now := setupContractionStore(t)

	result, err := s.MinimalContraction("r1", now)
	if err != nil {
		t.Fatalf("MinimalContraction failed: %v", err)
	}

	t.Logf("Explanation:\n%s", result.Explanation)

	// b1 depends only on r1 — must be removed
	assertContains(t, result.Removed, "b1", "b1 depends only on r1")
	// b4 depends only on b1 — must be removed
	assertContains(t, result.Removed, "b4", "b4 transitively depends only on r1 via b1")
	// b3 depends on r1 AND r2 — has clean path via r2, should be preserved
	assertNotContains(t, result.Removed, "b3", "b3 has clean path via r2")
	// b2 doesn't depend on r1 at all — preserved
	assertNotContains(t, result.Removed, "b2", "b2 has no dependency on r1")
}

func TestAGMVacuity(t *testing.T) {
	// K÷4: contracting a record with no dependent beliefs changes nothing
	s, now := setupContractionStore(t)

	// Add an isolated record with no beliefs
	s.Assert(&Record{ID: "r-isolated", Frame: "test", Content: "No beliefs derive from this", Timestamp: now})

	result, err := s.MinimalContraction("r-isolated", now)
	if err != nil {
		t.Fatalf("MinimalContraction failed: %v", err)
	}

	if len(result.Removed) != 0 {
		t.Errorf("expected no beliefs removed for isolated record, got %v", result.Removed)
	}
	t.Logf("Vacuity: %s", result.Explanation)
}

func TestAGMInclusion(t *testing.T) {
	// K÷3: Removed ⊆ original belief set
	s, now := setupContractionStore(t)

	result, err := s.MinimalContraction("r1", now)
	if err != nil {
		t.Fatalf("MinimalContraction failed: %v", err)
	}

	audit := s.PostulateAudit(result, "r1")
	for name, pr := range audit {
		if !pr.Passed {
			t.Errorf("Postulate %s failed: %s", name, pr.Note)
		}
		t.Logf("%s: %s — %s", name, boolStr(pr.Passed), pr.Note)
	}
}

func TestApplyContraction(t *testing.T) {
	s, now := setupContractionStore(t)

	result, err := s.MinimalContraction("r1", now)
	if err != nil {
		t.Fatalf("MinimalContraction failed: %v", err)
	}

	if err := s.ApplyContraction(result, "empirical finding A was incorrect", now); err != nil {
		t.Fatalf("ApplyContraction failed: %v", err)
	}

	// b1 and b4 should be contracted (BeliefSuperseded, ContractedBy set).
	// With soft-delete they remain in the store for recovery.
	r1b, err := s.Query("b1", now)
	if err != nil {
		t.Errorf("b1 should still be in store after soft-delete: %v", err)
	} else if r1b.State != BeliefSuperseded {
		t.Errorf("b1 should be BeliefSuperseded after contraction, got %v", r1b.State)
	}
	r4b, err := s.Query("b4", now)
	if err != nil {
		t.Errorf("b4 should still be in store after soft-delete: %v", err)
	} else if r4b.State != BeliefSuperseded {
		t.Errorf("b4 should be BeliefSuperseded after contraction, got %v", r4b.State)
	}
	// ContractedBy should point to the retracted record.
	s.mu.RLock()
	if b := s.beliefs["b1"]; b != nil && b.ContractedBy != "r1" {
		t.Errorf("b1.ContractedBy = %q, want r1", b.ContractedBy)
	}
	s.mu.RUnlock()

	// b2 and b3 should still exist
	if _, err := s.Query("b2", now); err != nil {
		t.Errorf("b2 should be preserved: %v", err)
	}
	if _, err := s.Query("b3", now); err != nil {
		t.Errorf("b3 should be preserved: %v", err)
	}

	// r1 should be retracted
	rec := s.records["r1"]
	if rec == nil || !rec.Retracted {
		t.Error("r1 should be marked retracted")
	}
}

func TestContractionPreservesCleanPaths(t *testing.T) {
	// b3 derived from both r1 and r2 — contracting r1 should keep b3
	s, now := setupContractionStore(t)

	result, err := s.MinimalContraction("r1", now)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	for _, id := range result.Preserved {
		if id == "b3" {
			t.Logf("b3 correctly preserved (has clean path via r2)")
			return
		}
	}
	t.Error("b3 should be in Preserved list")
}

func TestContractionChain(t *testing.T) {
	// Deep chain: r → b1 → b2 → b3, all must be removed
	s := NewStore()
	now := time.Now()
	f := Frame{Name: "test", Decay: DecayPolicy{Kind: "none"}}
	s.RegisterFrame(f)

	s.Assert(&Record{ID: "r", Frame: "test", Content: "Root record", Timestamp: now})
	s.Believe(&Belief{ID: "b1", Frame: "test", Content: "Level 1", Confidence: 0.9, AssertedAt: now, Derivation: []string{"r"}})
	s.Believe(&Belief{ID: "b2", Frame: "test", Content: "Level 2", Confidence: 0.8, AssertedAt: now, Derivation: []string{"b1"}})
	s.Believe(&Belief{ID: "b3", Frame: "test", Content: "Level 3", Confidence: 0.7, AssertedAt: now, Derivation: []string{"b2"}})

	result, err := s.MinimalContraction("r", now)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(result.Removed) != 3 {
		t.Errorf("expected 3 removed, got %d: %v", len(result.Removed), result.Removed)
	}
	t.Logf("Chain contraction: removed %v", result.Removed)
}

// helpers
func assertContains(t *testing.T, slice []string, item, reason string) {
	t.Helper()
	for _, s := range slice {
		if s == item {
			return
		}
	}
	t.Errorf("expected %q in %v (%s)", item, slice, reason)
}

func assertNotContains(t *testing.T, slice []string, item, reason string) {
	t.Helper()
	for _, s := range slice {
		if s == item {
			t.Errorf("expected %q NOT in %v (%s)", item, slice, reason)
			return
		}
	}
}

func boolStr(b bool) string {
	if b {
		return "PASS"
	}
	return "FAIL"
}

func TestContractionDiamond(t *testing.T) {
	// Diamond: r1 → b1, r1 → b2, b1+b2 → b3
	// Contracting on r1 should remove b1, b2, AND b3 (b3 has no clean path).
	// A single-pass scan would miss b3 if b1/b2 are processed after it.
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "test", Decay: DecayPolicy{Kind: "none"}})

	s.Assert(&Record{ID: "r1", Frame: "test", Content: "The only source", Timestamp: now})
	s.Believe(&Belief{ID: "b1", Frame: "test", Content: "From r1 path A", Confidence: 0.8, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "b2", Frame: "test", Content: "From r1 path B", Confidence: 0.7, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "b3", Frame: "test", Content: "From b1 and b2", Confidence: 0.9, AssertedAt: now, Derivation: []string{"b1", "b2"}})

	result, err := s.MinimalContraction("r1", now)
	if err != nil {
		t.Fatalf("MinimalContraction: %v", err)
	}
	t.Logf("Removed: %v", result.Removed)
	t.Logf("Preserved: %v", result.Preserved)

	// All three beliefs must be removed — b3 has no path independent of r1
	for _, id := range []string{"b1", "b2", "b3"} {
		found := false
		for _, r := range result.Removed {
			if r == id { found = true; break }
		}
		if !found {
			t.Errorf("belief %s should be in removed set (all paths go through r1), got removed=%v", id, result.Removed)
		}
	}
}

func TestContractionDiamondWithCleanPath(t *testing.T) {
	// Diamond with clean escape: r1 → b1, r1 → b2, r2 → b2, b1+b2 → b3
	// b2 has r2 as a clean path, so b2 and b3 should be preserved.
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "test", Decay: DecayPolicy{Kind: "none"}})

	s.Assert(&Record{ID: "r1", Frame: "test", Content: "Retracted source", Timestamp: now})
	s.Assert(&Record{ID: "r2", Frame: "test", Content: "Clean source", Timestamp: now})
	s.Believe(&Belief{ID: "b1", Frame: "test", Content: "Only from r1", Confidence: 0.8, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "b2", Frame: "test", Content: "From r1 and r2", Confidence: 0.7, AssertedAt: now, Derivation: []string{"r1", "r2"}})
	s.Believe(&Belief{ID: "b3", Frame: "test", Content: "From b1 and b2", Confidence: 0.9, AssertedAt: now, Derivation: []string{"b1", "b2"}})

	result, err := s.MinimalContraction("r1", now)
	if err != nil {
		t.Fatalf("MinimalContraction: %v", err)
	}
	t.Logf("Removed: %v", result.Removed)
	t.Logf("Preserved: %v", result.Preserved)

	// b1 must be removed (sole source is r1)
	b1Removed := false
	for _, r := range result.Removed { if r == "b1" { b1Removed = true; break } }
	if !b1Removed { t.Error("b1 should be removed (sole source is r1)") }

	// b2 has r2 as clean path — should be preserved
	b2Preserved := false
	for _, p := range result.Preserved { if p == "b2" { b2Preserved = true; break } }
	if !b2Preserved { t.Error("b2 should be preserved (has clean path via r2)") }

	// b3 derives from b1 (removed) and b2 (preserved) — b2 is clean, so b3 has a clean path
	b3Preserved := false
	for _, p := range result.Preserved { if p == "b3" { b3Preserved = true; break } }
	if !b3Preserved { t.Error("b3 should be preserved (b2 is a clean parent)") }
}
