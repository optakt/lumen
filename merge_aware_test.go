package lumen

import (
	"math"
	"testing"
	"time"
)

func TestCorrelationAwareMerge(t *testing.T) {
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayNone}})

	// CASE 1: Independent sources — should approach standard noisy-or
	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "Cogitate study", Timestamp: now})
	s.Assert(&Record{ID: "r2", Frame: "empirical", Content: "Independent replication", Timestamp: now})
	s.Believe(&Belief{ID: "bA", Frame: "empirical", Content: "IIT weakened by Cogitate", Confidence: 0.70, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "bB", Frame: "empirical", Content: "IIT weakened by replication", Confidence: 0.65, AssertedAt: now, Derivation: []string{"r2"}})

	resultIndep, err := s.CorrelationAwareMerge("bA", "bB", "merged-indep",
		"IIT is substantially weakened.", "empirical", false, now)
	if err != nil { t.Fatalf("CorrelationAwareMerge (independent): %v", err) }

	noisyOr := 1 - (1-0.70)*(1-0.65) // = 0.895
	t.Logf("Independent merge: %s", resultIndep.CombinationMethod)
	t.Logf("Noisy-or would give: %.3f, got: %.3f", noisyOr, resultIndep.CombinedConfidence)
	// With independent sources, correlation-aware ≈ standard noisy-or (within 5%)
	if math.Abs(resultIndep.CombinedConfidence-noisyOr) > 0.05 {
		t.Errorf("independent sources should approximate noisy-or: got %.3f vs %.3f",
			resultIndep.CombinedConfidence, noisyOr)
	}

	// CASE 2: Shared sources — should be lower than noisy-or
	s.Believe(&Belief{ID: "bC", Frame: "empirical", Content: "IIT faces empirical challenges", Confidence: 0.72, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "bD", Frame: "empirical", Content: "IIT predictions unconfirmed", Confidence: 0.68, AssertedAt: now, Derivation: []string{"r1"}})

	resultShared, err := s.CorrelationAwareMerge("bC", "bD", "merged-shared",
		"IIT is challenged by empirical evidence.", "empirical", false, now)
	if err != nil { t.Fatalf("CorrelationAwareMerge (shared): %v", err) }

	noisyOrShared := 1 - (1-0.72)*(1-0.68) // = 0.9104
	t.Logf("Shared-source merge: %s", resultShared.CombinationMethod)
	t.Logf("Noisy-or would give: %.3f, correlation-aware gives: %.3f",
		noisyOrShared, resultShared.CombinedConfidence)
	// With shared sources, should be less than standard noisy-or
	if resultShared.CombinedConfidence >= noisyOrShared {
		t.Errorf("shared sources should give less than noisy-or: %.3f >= %.3f",
			resultShared.CombinedConfidence, noisyOrShared)
	}
}

func TestDerivationOverlap(t *testing.T) {
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayNone}})

	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "Shared source", Timestamp: now})
	s.Assert(&Record{ID: "r2", Frame: "empirical", Content: "Unique to A", Timestamp: now})
	s.Assert(&Record{ID: "r3", Frame: "empirical", Content: "Unique to B", Timestamp: now})

	// bA uses r1+r2, bB uses r1+r3 — 50% overlap (r1 is shared)
	s.Believe(&Belief{ID: "bA", Frame: "empirical", Content: "From r1+r2", Confidence: 0.7, AssertedAt: now, Derivation: []string{"r1", "r2"}})
	s.Believe(&Belief{ID: "bB", Frame: "empirical", Content: "From r1+r3", Confidence: 0.7, AssertedAt: now, Derivation: []string{"r1", "r3"}})

	overlap, err := s.DerivationOverlap("bA", "bB", now)
	if err != nil { t.Fatalf("DerivationOverlap: %v", err) }
	// Jaccard: |{r1}| / |{r1,r2,r3}| = 1/3 ≈ 0.333
	t.Logf("Derivation overlap (1 shared of 3): %.3f", overlap)
	expected := 1.0 / 3.0
	if math.Abs(overlap-expected) > 0.01 {
		t.Errorf("expected Jaccard overlap %.3f, got %.3f", expected, overlap)
	}

	// bC uses r1+r2 (identical to bA) — full overlap
	s.Believe(&Belief{ID: "bC", Frame: "empirical", Content: "Identical sources", Confidence: 0.7, AssertedAt: now, Derivation: []string{"r1", "r2"}})
	fullOverlap, _ := s.DerivationOverlap("bA", "bC", now)
	t.Logf("Full overlap: %.3f", fullOverlap)
	if math.Abs(fullOverlap-1.0) > 0.001 {
		t.Errorf("identical sources should give overlap 1.0, got %.3f", fullOverlap)
	}
}

func TestEvidenceMatrix(t *testing.T) {
	s, now := setupProvenanceStore(t)
	ids, matrix := s.EvidenceMatrix(now)
	t.Logf("Evidence matrix (%d beliefs):", len(ids))
	t.Log(RenderEvidenceMatrix(ids, matrix))

	// Diagonal should be 1
	for i := range ids {
		if matrix[i][i] != 1.0 {
			t.Errorf("diagonal[%d] should be 1.0, got %.2f", i, matrix[i][i])
		}
	}
	// b1 and b2 both derive from different records — no overlap
	// b3 derives from b1+b2 — overlap with both
}
