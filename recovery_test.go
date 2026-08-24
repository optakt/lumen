package lumen

import (
	"testing"
	"time"
)

// setupContractionStore creates a store with a known contraction scenario:
//
//	r1 ──► b1 ──► b3 (b3 depends exclusively on b1)
//	r2 ──► b2 ──►┘   (b2 has alternative clean path via r2)
//
// Retracting r1 should remove b1 and b3 (both depend exclusively on r1 path),
// but preserve b2 (clean path via r2 remains).
func setupRecoveryStore(t *testing.T) (*Store, time.Time) {
	t.Helper()
	s := NewStore()
	s.RegisterFrame(Frame{Name: "test", Decay: DecayPolicy{Kind: DecayNone}})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	must := func(err error) {
		t.Helper()
		if err != nil { t.Fatalf("setup: %v", err) }
	}
	must(s.Assert(&Record{ID: "r1", Content: "R1.", Frame: "test", Timestamp: now}))
	must(s.Assert(&Record{ID: "r2", Content: "R2.", Frame: "test", Timestamp: now}))
	must(s.Believe(&Belief{ID: "b1", Content: "From r1.", Confidence: 0.80, Frame: "test", AssertedAt: now, Derivation: []string{"r1"}}))
	must(s.Believe(&Belief{ID: "b2", Content: "From r1 and r2.", Confidence: 0.75, Frame: "test", AssertedAt: now, Derivation: []string{"r1", "r2"}}))
	must(s.Believe(&Belief{ID: "b3", Content: "From b1 only.", Confidence: 0.70, Frame: "test", AssertedAt: now, Derivation: []string{"b1"}}))
	return s, now
}

func TestRecoverAfterContraction(t *testing.T) {
	s, now := setupRecoveryStore(t)
	t1 := now.Add(time.Hour)

	// Contract on r1 retraction.
	result, err := s.MinimalContraction("r1", now)
	if err != nil {
		t.Fatalf("MinimalContraction: %v", err)
	}
	t.Logf("contraction removes: %v, preserves: %v", result.Removed, result.Preserved)

	if err := s.ApplyContraction(result, "r1 retracted for test", t1); err != nil {
		t.Fatalf("ApplyContraction: %v", err)
	}

	// After contraction: b1 and b3 should be BeliefSuperseded.
	s.mu.RLock()
	b1 := s.beliefs["b1"]
	b3 := s.beliefs["b3"]
	s.mu.RUnlock()

	if b1 == nil || b1.State != BeliefSuperseded {
		t.Errorf("b1 should be BeliefSuperseded after contraction, got %v", b1)
	}
	if b3 == nil || b3.State != BeliefSuperseded {
		t.Errorf("b3 should be BeliefSuperseded after contraction, got %v", b3)
	}
	if b1 != nil && b1.ContractedBy != "r1" {
		t.Errorf("b1.ContractedBy should be r1, got %q", b1.ContractedBy)
	}

	// ContractedBeliefs should include b1 and b3.
	contracted := s.ContractedBeliefs()
	contractedSet := make(map[string]bool)
	for _, id := range contracted {
		contractedSet[id] = true
	}
	if !contractedSet["b1"] || !contractedSet["b3"] {
		t.Errorf("expected b1 and b3 in contracted beliefs, got %v", contracted)
	}
	t.Logf("contracted beliefs: %v", contracted)

	// Recover fails while r1 is still retracted.
	t2 := t1.Add(time.Hour)
	if err := s.Recover("b1", t2); err == nil {
		t.Error("expected Recover to fail while r1 is retracted")
	} else {
		t.Logf("expected failure: %v", err)
	}

	// Re-assert r1 (un-retract by replacing the record).
	s.mu.Lock()
	if rec, ok := s.records["r1"]; ok {
		rec.Retracted = false
		rec.RetractedAt = time.Time{}
		rec.RetractReason = ""
	}
	s.mu.Unlock()

	// Now recovery should succeed for b1 (b3 depends on b1, which is still superseded).
	t3 := t2.Add(time.Hour)
	if err := s.Recover("b1", t3); err != nil {
		t.Fatalf("Recover b1: %v", err)
	}
	s.mu.RLock()
	b1After := s.beliefs["b1"]
	s.mu.RUnlock()
	if b1After.State != BeliefActive {
		t.Errorf("b1 should be BeliefActive after recovery, got %v", b1After.State)
	}
	t.Logf("b1 recovered: state=%v, ContractedBy=%q", b1After.State, b1After.ContractedBy)

	// Now b3 can also be recovered (b1 is active again).
	if err := s.Recover("b3", t3); err != nil {
		t.Fatalf("Recover b3: %v", err)
	}
	s.mu.RLock()
	b3After := s.beliefs["b3"]
	s.mu.RUnlock()
	if b3After.State != BeliefActive {
		t.Errorf("b3 should be BeliefActive after recovery, got %v", b3After.State)
	}
	t.Logf("b3 recovered: state=%v", b3After.State)
}

func TestRecoverNonContractedBelief(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "test", Decay: DecayPolicy{Kind: DecayNone}})
	now := time.Now()
	_ = s.Assert(&Record{ID: "r1", Content: "R.", Frame: "test", Timestamp: now})
	_ = s.Believe(&Belief{ID: "b1", Content: "B.", Confidence: 0.80, Frame: "test", AssertedAt: now, Derivation: []string{"r1"}})

	err := s.Recover("b1", now)
	if err == nil {
		t.Error("expected error when recovering non-contracted belief")
	}
	t.Logf("expected error: %v", err)
}

func TestRecoverNotFound(t *testing.T) {
	s := NewStore()
	err := s.Recover("ghost", time.Now())
	if err == nil {
		t.Error("expected error for missing belief")
	}
}

func TestPostulateAuditK5(t *testing.T) {
	s, now := setupRecoveryStore(t)
	result, err := s.MinimalContraction("r1", now)
	if err != nil {
		t.Fatalf("MinimalContraction: %v", err)
	}

	audit := s.PostulateAudit(result, "r1")

	k5, ok := audit["K÷5"]
	if !ok {
		t.Fatal("PostulateAudit should include K÷5 (Recovery)")
	}
	if !k5.Passed {
		t.Errorf("K÷5 should pass: %s", k5.Note)
	}
	t.Logf("K÷5: %s", k5.Note)
	_ = now
}

func TestContractedBeliefsEmpty(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: DecayNone}})
	now := time.Now()
	_ = s.Assert(&Record{ID: "r1", Content: "R.", Frame: "f", Timestamp: now})
	_ = s.Believe(&Belief{ID: "b1", Content: "B.", Confidence: 0.80, Frame: "f", AssertedAt: now, Derivation: []string{"r1"}})

	contracted := s.ContractedBeliefs()
	if len(contracted) != 0 {
		t.Errorf("expected 0 contracted beliefs in fresh store, got %v", contracted)
	}
}
