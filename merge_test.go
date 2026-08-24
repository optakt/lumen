package lumen

import (
	"math"
	"testing"
	"time"
)

func TestMergeBeliefs(t *testing.T) {
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayNone}})

	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "Cogitate study", Timestamp: now})
	s.Assert(&Record{ID: "r2", Frame: "empirical", Content: "Independent replication", Timestamp: now})
	s.Believe(&Belief{ID: "bA", Frame: "empirical", Content: "IIT is weakened by Cogitate", Confidence: 0.70, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "bB", Frame: "empirical", Content: "IIT is further weakened by replication failure", Confidence: 0.65, AssertedAt: now, Derivation: []string{"r2"}})

	result, err := s.MergeBeliefs("bA", "bB", "iit-weakened-merged",
		"IIT is substantially weakened by Cogitate results and independent replication failure.",
		"empirical", "noisy-or", false, now)
	if err != nil { t.Fatalf("MergeBeliefs: %v", err) }

	t.Logf("Merged: %s", result.CombinationMethod)
	t.Logf("Combined confidence: %.3f (noisy-or of %.2f and %.2f)", result.CombinedConfidence, 0.70, 0.65)

	// noisy-or(0.70, 0.65) = 1 - (1-0.70)*(1-0.65) = 1 - 0.30*0.35 = 1 - 0.105 = 0.895
	expected := 1 - (1-0.70)*(1-0.65)
	if math.Abs(result.CombinedConfidence-expected) > 0.001 {
		t.Errorf("expected confidence %.3f, got %.3f", expected, result.CombinedConfidence)
	}

	// Merged belief should exist and derive from both original chains
	q, err := s.Query("iit-weakened-merged", now)
	if err != nil { t.Fatalf("Query merged: %v", err) }
	t.Logf("Merged belief: %q conf=%.2f", q.Content, q.CurrentConfidence)

	// Derivation is {bA, bB} only — r1/r2 are reachable transitively.
	s.mu.RLock()
	merged := s.beliefs["iit-weakened-merged"]
	s.mu.RUnlock()
	derivSet := make(map[string]bool)
	for _, id := range merged.Derivation { derivSet[id] = true }
	for _, expected := range []string{"bA", "bB"} {
		if !derivSet[expected] {
			t.Errorf("merged derivation missing %s: %v", expected, merged.Derivation)
		}
	}
	for _, unexpected := range []string{"r1", "r2"} {
		if derivSet[unexpected] {
			t.Errorf("merged derivation should not include %s directly: %v", unexpected, merged.Derivation)
		}
	}
}

func TestMergeCombinationMethods(t *testing.T) {
	cases := []struct {
		method   string
		pA, pB   float64
		expected float64
		tol      float64
	}{
		{"noisy-or",   0.7, 0.6, 1 - (1-0.7)*(1-0.6), 0.001},
		{"conservative", 0.7, 0.6, 0.6, 0.001},
		{"geometric",  0.7, 0.6, math.Sqrt(0.7 * 0.6), 0.001},
		{"average",    0.7, 0.6, 0.65, 0.001},
		{"bayesian",   0.7, 0.6, (0.6 * 0.7) / (0.6*0.7 + 0.4*0.3), 0.001},
	}
	for _, tc := range cases {
		got, desc := combineConfidence(tc.pA, tc.pB, tc.method)
		if math.Abs(got-tc.expected) > tc.tol {
			t.Errorf("%s: expected %.4f, got %.4f", tc.method, tc.expected, got)
		}
		t.Logf("%s: %s", tc.method, desc)
	}
}

func TestMergeWithRetire(t *testing.T) {
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "test", Decay: DecayPolicy{Kind: DecayNone}})

	s.Assert(&Record{ID: "r1", Frame: "test", Content: "Source A", Timestamp: now})
	s.Assert(&Record{ID: "r2", Frame: "test", Content: "Source B", Timestamp: now})
	s.Believe(&Belief{ID: "bA", Frame: "test", Content: "Claim from A", Confidence: 0.75, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "bB", Frame: "test", Content: "Claim from B", Confidence: 0.70, AssertedAt: now, Derivation: []string{"r2"}})

	_, err := s.MergeBeliefs("bA", "bB", "merged", "Synthesized claim.", "test", "average", true, now)
	if err != nil { t.Fatalf("MergeBeliefs: %v", err) }

	// Original beliefs should be suspect/superseded
	s.mu.RLock()
	bA := s.beliefs["bA"]
	bB := s.beliefs["bB"]
	s.mu.RUnlock()

	if bA.State != BeliefSuperseded { t.Error("bA should be superseded after retire") }
	if bB.State != BeliefSuperseded { t.Error("bB should be superseded after retire") }
	t.Logf("Retired bA state: %v content: %q", bA.State, bA.Content[:min4(20, len(bA.Content))])
}

func min4(a, b int) int {
	if a < b { return a }
	return b
}
