package lumen

import (
	"testing"
	"time"
)

func TestConflictDeclared(t *testing.T) {
	s := NewStore()
	now := time.Now()
	f := Frame{Name: "philosophical", Decay: DecayPolicy{Kind: DecayNone}}
	s.RegisterFrame(f)

	s.Assert(&Record{ID: "r1", Frame: "philosophical", Content: "Chalmers argues consciousness is irreducible", Timestamp: now})
	s.Assert(&Record{ID: "r2", Frame: "philosophical", Content: "Dennett argues consciousness is an illusion", Timestamp: now})
	s.Believe(&Belief{ID: "hard-problem", Frame: "philosophical", Content: "The hard problem of consciousness is real and irreducible", Confidence: 0.72, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "illusionism", Frame: "philosophical", Content: "Consciousness as we conceive it is an illusion; only functional states exist", Confidence: 0.25, AssertedAt: now, Derivation: []string{"r2"}})

	// Declare contrast explicitly
	s.Reference("hard-problem", "illusionism", EdgeContrasts, "opposing views")

	// Register entities so co-mention works
	s.Entities.RegisterEntity(&Entity{ID: "consciousness"})
	s.Entities.AddMention("hard-problem", "consciousness", "consciousness")
	s.Entities.AddMention("illusionism", "consciousness", "consciousness")

	conflicts := s.ConflictScan(now)
	if len(conflicts) == 0 {
		t.Fatal("expected at least one conflict")
	}
	found := false
	for _, c := range conflicts {
		if (c.BeliefA == "hard-problem" && c.BeliefB == "illusionism") ||
			(c.BeliefA == "illusionism" && c.BeliefB == "hard-problem") {
			found = true
			t.Logf("Declared conflict found: kind=%s strength=%.2f", c.Kind, c.Strength)
			t.Logf("  %s", c.Explanation)
		}
	}
	if !found {
		t.Error("declared contrast between hard-problem and illusionism not found in conflicts")
	}
}

func TestConflictNegation(t *testing.T) {
	s := NewStore()
	now := time.Now()
	f := Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayNone}}
	s.RegisterFrame(f)

	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "Study A confirms the hypothesis", Timestamp: now})
	s.Assert(&Record{ID: "r2", Frame: "empirical", Content: "Study B refutes the hypothesis", Timestamp: now})
	s.Believe(&Belief{ID: "b-confirm", Frame: "empirical", Content: "The hypothesis is confirmed by empirical evidence", Confidence: 0.8, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "b-refute", Frame: "empirical", Content: "The hypothesis is not confirmed and fails to replicate", Confidence: 0.7, AssertedAt: now, Derivation: []string{"r2"}})

	// Register shared entity
	s.Entities.RegisterEntity(&Entity{ID: "hypothesis"})
	s.Entities.AddMention("b-confirm", "hypothesis", "hypothesis")
	s.Entities.AddMention("b-refute", "hypothesis", "hypothesis")

	conflicts := s.ConflictScan(now)
	found := false
	for _, c := range conflicts {
		if (c.BeliefA == "b-confirm" || c.BeliefB == "b-confirm") &&
			(c.BeliefA == "b-refute" || c.BeliefB == "b-refute") {
			found = true
			t.Logf("Negation conflict found: kind=%s strength=%.2f", c.Kind, c.Strength)
			t.Logf("  %s", c.Explanation)
		}
	}
	if !found {
		t.Error("negation conflict between b-confirm and b-refute not detected")
	}
}

func TestConflictNoFalsePositives(t *testing.T) {
	// Two beliefs about the same entity that don't conflict
	s := NewStore()
	now := time.Now()
	f := Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayNone}}
	s.RegisterFrame(f)

	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "Gödel published his theorems in 1931", Timestamp: now})
	s.Assert(&Record{ID: "r2", Frame: "empirical", Content: "Gödel's theorems apply to formal systems", Timestamp: now})
	s.Believe(&Belief{ID: "b1", Frame: "empirical", Content: "Gödel's incompleteness results are foundational to logic", Confidence: 0.95, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "b2", Frame: "empirical", Content: "Gödel's theorems constrain the limits of formal provability", Confidence: 0.90, AssertedAt: now, Derivation: []string{"r2"}})

	s.Entities.RegisterEntity(&Entity{ID: "godel", Aliases: []string{"Gödel", "Goedel"}})
	s.Entities.AddMention("b1", "godel", "Gödel")
	s.Entities.AddMention("b2", "godel", "Gödel")

	conflicts := s.ConflictScan(now)
	for _, c := range conflicts {
		if c.Kind == "negation" &&
			((c.BeliefA == "b1" && c.BeliefB == "b2") ||
				(c.BeliefA == "b2" && c.BeliefB == "b1")) {
			t.Errorf("false positive negation conflict between b1 and b2: %s", c.Explanation)
		}
	}
	t.Logf("No false positive negation conflicts (found %d total conflicts)", len(conflicts))
}
