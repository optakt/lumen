package lumen

import (
	"testing"
	"time"
)

func TestValidateCleanStore(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: "none"}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = s.Assert(&Record{ID: "r1", Frame: "f", Content: "A.", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "b1", Frame: "f", Content: "B.", Confidence: 0.80, AssertedAt: t0, Derivation: []string{"r1"}})

	issues := s.Validate()
	if len(issues) != 0 {
		for _, i := range issues { t.Errorf("unexpected issue: %s", i) }
	}
}

func TestValidateOrphanedReference(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: "none"}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Inject directly — Believe rejects bad references at insertion time.
	s.beliefs["b1"] = &Belief{ID: "b1", Frame: "f", Content: "B.", Confidence: 0.80, AssertedAt: t0, Derivation: []string{"nonexistent"}}

	issues := s.Validate()
	found := false
	for _, i := range issues {
		if i.BeliefID == "b1" && i.Kind == "error" { found = true }
	}
	if !found { t.Error("should detect orphaned derivation reference") }
}

func TestValidateUndefinedFrame(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Inject directly — Believe rejects unknown frames at insertion time.
	s.beliefs["b1"] = &Belief{ID: "b1", Frame: "unknown", Content: "B.", Confidence: 0.50, AssertedAt: t0}

	issues := s.Validate()
	found := false
	for _, i := range issues {
		if i.BeliefID == "b1" && i.Kind == "error" { found = true }
	}
	if !found { t.Error("should detect undefined frame reference") }
}

func TestValidateCircularDerivation(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: "none"}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Inject circular reference directly (bypassing normal Believe validation).
	s.beliefs["b1"] = &Belief{ID: "b1", Frame: "f", Content: "A.", Confidence: 0.80, AssertedAt: t0, Derivation: []string{"b2"}}
	s.beliefs["b2"] = &Belief{ID: "b2", Frame: "f", Content: "B.", Confidence: 0.70, AssertedAt: t0, Derivation: []string{"b1"}}

	issues := s.Validate()
	found := false
	for _, i := range issues {
		if i.Kind == "error" && len(i.Message) > 0 { found = true }
		t.Logf("issue: %s", i)
	}
	if !found { t.Error("should detect circular derivation") }
}
