package lumen

import (
	"testing"
	"time"
)

func TestReviseBasic(t *testing.T) {
	s := NewStore()
	now := time.Now()
	f := Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayNone}}
	s.RegisterFrame(f)

	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "Study found X causes Y with p=0.03", Timestamp: now})
	s.Assert(&Record{ID: "r2", Frame: "empirical", Content: "Independent replication", Timestamp: now})
	s.Believe(&Belief{ID: "b1", Frame: "empirical", Content: "X causes Y", Confidence: 0.8, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "b2", Frame: "empirical", Content: "X and Y are causally linked", Confidence: 0.75, AssertedAt: now, Derivation: []string{"r1", "r2"}})
	s.Believe(&Belief{ID: "b3", Frame: "empirical", Content: "Unrelated conclusion", Confidence: 0.9, AssertedAt: now, Derivation: []string{"r2"}})

	// Revise r1: the study failed to replicate, update its content
	result, err := s.Revise("r1", "Study found X correlates with Y (p=0.03) but replication failed", 0, "failed replication", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Revise failed: %v", err)
	}

	t.Logf("Revised: %q → %q", result.OldContent, result.NewContent)
	t.Logf("Suspect beliefs: %v", result.Suspect)
	t.Logf("Unchanged beliefs: %v", result.Unchanged)

	// b1 and b2 depend on r1 — both should be suspect
	foundB1, foundB2 := false, false
	for _, id := range result.Suspect {
		if id == "b1" { foundB1 = true }
		if id == "b2" { foundB2 = true }
	}
	if !foundB1 { t.Error("b1 should be suspect after revising r1") }
	if !foundB2 { t.Error("b2 should be suspect after revising r1") }

	// b3 only depends on r2 — unchanged
	foundB3 := false
	for _, id := range result.Unchanged {
		if id == "b3" { foundB3 = true }
	}
	if !foundB3 { t.Error("b3 should be unchanged") }

	// b1 and b2 should be in suspect state in the store
	qb1, err := s.Query("b1", now.Add(time.Hour))
	if err != nil { t.Fatalf("Query b1: %v", err) }
	if qb1.State != BeliefSuspect { t.Errorf("b1 should be suspect, got %v", qb1.State) }
}

func TestReviseAndReAssert(t *testing.T) {
	s := NewStore()
	now := time.Now()
	f := Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayNone}}
	s.RegisterFrame(f)

	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "Original finding", Timestamp: now})
	s.Believe(&Belief{ID: "b1", Frame: "empirical", Content: "Conclusion from original", Confidence: 0.8, AssertedAt: now, Derivation: []string{"r1"}})

	_, err := s.Revise("r1", "Corrected finding", 0, "correction", now.Add(time.Hour))
	if err != nil { t.Fatalf("Revise: %v", err) }

	// b1 is now suspect — re-assert it with updated content and confidence
	err = s.ReAssert("b1", "Conclusion from corrected finding", 0.65, now.Add(time.Hour))
	if err != nil { t.Fatalf("ReAssert: %v", err) }

	q, err := s.Query("b1", now.Add(time.Hour))
	if err != nil { t.Fatalf("Query: %v", err) }

	if q.State != BeliefActive { t.Errorf("b1 should be active after re-assertion, got %v", q.State) }
	if q.CurrentConfidence < 0.64 || q.CurrentConfidence > 0.66 {
		t.Errorf("expected confidence ~0.65, got %.3f", q.CurrentConfidence)
	}
	t.Logf("Re-asserted b1: state=%v confidence=%.2f content=%q", q.State, q.CurrentConfidence, q.Content)
}

func TestReviseAGMVacuity(t *testing.T) {
	// K*4: if the revision doesn't conflict with anything, result = expansion
	// In Lumen terms: revising a record with no dependent beliefs produces no suspect beliefs
	s := NewStore()
	now := time.Now()
	f := Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayNone}}
	s.RegisterFrame(f)

	s.Assert(&Record{ID: "r-solo", Frame: "empirical", Content: "Isolated record", Timestamp: now})
	result, err := s.Revise("r-solo", "Updated isolated record", 0, "", now.Add(time.Hour))
	if err != nil { t.Fatalf("Revise: %v", err) }

	if len(result.Suspect) != 0 {
		t.Errorf("expected no suspect beliefs for isolated revision, got %v", result.Suspect)
	}
	t.Log("Vacuity: no beliefs affected by revising isolated record")
}
