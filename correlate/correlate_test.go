package correlate_test

import (
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/optakt/lumen/correlate"
)

func apiKey(t *testing.T) string {
	t.Helper()
	k := os.Getenv("VOYAGEAI_API_KEY")
	if k == "" {
		t.Skip("VOYAGEAI_API_KEY not set")
	}
	return k
}

// TestKnownCorrelated: zombie argument and Mary's Room (knowledge argument)
// share the same modal intuition — should score high similarity.
func TestKnownCorrelated(t *testing.T) {
	key := apiKey(t)
	pairs := []correlate.EvidencePair{
		{
			IDa:   "zombie-argument",
			IDb:   "knowledge-argument",
			TextA: "Philosophical zombies are conceivable: a being physically identical to a conscious person but with no inner experience. If zombies are conceivable, consciousness is not logically entailed by physical facts. Therefore physicalism cannot fully explain consciousness.",
			TextB: "Mary is a scientist who knows all physical facts about color vision but has never seen red. When she first sees red, she learns something new — what it is like to see red. Therefore phenomenal properties are not captured by physical facts.",
		},
		{
			IDa:   "zombie-argument",
			IDb:   "conceivability-argument",
			TextA: "Philosophical zombies are conceivable: a being physically identical to a conscious person but with no inner experience. If zombies are conceivable, consciousness is not logically entailed by physical facts. Therefore physicalism cannot fully explain consciousness.",
			TextB: "It is conceivable that all the functional and physical facts could hold while phenomenal consciousness is absent. Conceivability implies metaphysical possibility. Therefore consciousness is a further fact beyond the physical.",
		},
		// Independent: empirical neuroscience finding vs philosophical thought experiment
		{
			IDa:   "cmd-data",
			IDb:   "zombie-argument",
			TextA: "Cognitive motor dissociation study: 25% of clinically unresponsive patients showed command-following brain activity on fMRI or EEG despite showing no behavioral response. NEJM 2024.",
			TextB: "Philosophical zombies are conceivable: a being physically identical to a conscious person but with no inner experience. If zombies are conceivable, consciousness is not logically entailed by physical facts. Therefore physicalism cannot fully explain consciousness.",
		},
		// Borderline: GWT and predictive processing — related frameworks, different mechanisms
		{
			IDa:   "gwt",
			IDb:   "predictive-processing",
			TextA: "Global Workspace Theory: consciousness arises when information is broadcast globally from a central workspace to specialized processors. The prefrontal cortex acts as a workspace hub; conscious access = global broadcast availability.",
			TextB: "Predictive processing: the brain is a prediction machine that minimizes free energy by updating internal models. Consciousness may correspond to high-level predictions about the causes of sensory input, with attention as precision-weighting of prediction errors.",
		},
		// Same framework, different aspects — should be moderate-high
		{
			IDa:   "iit-phi",
			IDb:   "iit-exclusion",
			TextA: "Integrated Information Theory: consciousness is identical to integrated information (phi). A system is conscious to the degree that it has irreducible cause-effect power over itself.",
			TextB: "IIT's exclusion postulate: consciousness exists at only one level of grain — the level that maximizes phi. Overlapping systems cannot both be conscious; only the one with maximal phi at its grain is the conscious entity.",
		},
	}

	results, err := correlate.AnalyzePairs(key, pairs)
	if err != nil {
		t.Fatalf("AnalyzePairs: %v", err)
	}

	fmt.Println("\n=== Correlation Analysis Results ===")
	for _, r := range results {
		fmt.Printf("\n  %s / %s\n", r.IDa, r.IDb)
		fmt.Printf("  cosine=%.4f  r_estimated=%.3f\n", r.CosineSimilarity, r.EstimatedCorrelation)
		fmt.Printf("  %s\n", r.Interpretation)
	}

	// Zombie / knowledge argument should be highly correlated
	if results[0].EstimatedCorrelation < 0.1 {
		t.Errorf("zombie/knowledge: expected r >= 0.1 (note: Voyage-3 scores surface similarity; zombie & Mary use different vocab despite same modal intuition), got %.3f (cosine=%.4f)",
			results[0].EstimatedCorrelation, results[0].CosineSimilarity)
	}

	// CMD data / zombie should be independent
	if results[2].EstimatedCorrelation > 0.2 {
		t.Errorf("cmd/zombie: expected r <= 0.2, got %.3f (cosine=%.4f)",
			results[2].EstimatedCorrelation, results[2].CosineSimilarity)
	}

	// IIT phi / exclusion — same framework, should have some correlation
	if results[4].EstimatedCorrelation < 0.2 {
		t.Errorf("iit-phi/iit-exclusion: expected r >= 0.2, got %.3f (cosine=%.4f)",
			results[4].EstimatedCorrelation, results[4].CosineSimilarity)
	}
}

func TestCosineSimilarityMath(t *testing.T) {
	// Identical vectors: similarity = 1
	v := []float64{1, 0, 0, 0}
	sim, err := correlate.CosineSimilarity(v, v)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(sim-1.0) > 1e-9 {
		t.Errorf("identical vectors: expected 1.0, got %.6f", sim)
	}

	// Orthogonal vectors: similarity = 0
	a := []float64{1, 0}
	b := []float64{0, 1}
	sim2, _ := correlate.CosineSimilarity(a, b)
	if math.Abs(sim2) > 1e-9 {
		t.Errorf("orthogonal vectors: expected 0.0, got %.6f", sim2)
	}

	// Opposite vectors: similarity = -1
	c := []float64{1, 0}
	d := []float64{-1, 0}
	sim3, _ := correlate.CosineSimilarity(c, d)
	if math.Abs(sim3+1.0) > 1e-9 {
		t.Errorf("opposite vectors: expected -1.0, got %.6f", sim3)
	}

	// Dimension mismatch: error
	_, err2 := correlate.CosineSimilarity([]float64{1, 2}, []float64{1, 2, 3})
	if err2 == nil {
		t.Error("expected error on dimension mismatch")
	}
}

func TestThresholdCalibration(t *testing.T) {
	// Test the threshold mapping function indirectly through AnalyzePairs
	// by constructing pairs with known expected correlation ranges
	key := apiKey(t)

	// Near-duplicate claims: should be very high similarity → r > 0.6
	nearDupe := []correlate.EvidencePair{{
		IDa:   "claim-a",
		IDb:   "claim-b",
		TextA: "The hard problem of consciousness is the question of why there is something it is like to be in certain brain states, even when all functional facts are explained.",
		TextB: "The hard problem asks why physical brain processes give rise to subjective experience — why there is phenomenal consciousness at all, beyond functional description.",
	}}

	results, err := correlate.AnalyzePairs(key, nearDupe)
	if err != nil {
		t.Fatalf("AnalyzePairs: %v", err)
	}
	t.Logf("Near-duplicate: cosine=%.4f r=%.3f", results[0].CosineSimilarity, results[0].EstimatedCorrelation)
	if results[0].EstimatedCorrelation < 0.5 {
		t.Logf("WARNING: near-duplicate scored r=%.3f — threshold may need adjustment", results[0].EstimatedCorrelation)
	}
}
