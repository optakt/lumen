package lumen

import (
	"fmt"
	"math"
	"testing"
)

func TestBayesianComposeBasic(t *testing.T) {
	// Single strong positive evidence
	posterior, err := BayesianCompose(0.1, []Evidence{
		{SourceID: "e1", Confidence: 0.95, LikelihoodRatio: 10.0},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Prior 0.1 + LR=10 (conf=0.95) → posterior %.4f", posterior)
	// Expected: log_odds(0.1) + log(1 + 0.95*(10-1)) = log(-2.197) + log(9.55) ≈ -2.197 + 2.256 = 0.059
	// posterior = 1/(1+e^-0.059) ≈ 0.515
	if posterior < 0.45 || posterior > 0.60 {
		t.Errorf("expected ~0.515, got %.4f", posterior)
	}

	// Two independent pieces of supporting evidence
	posterior2, err := BayesianCompose(0.1, []Evidence{
		{SourceID: "e1", Confidence: 0.9, LikelihoodRatio: 5.0},
		{SourceID: "e2", Confidence: 0.85, LikelihoodRatio: 4.0},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Prior 0.1 + two supporting (LR=5, LR=4) → posterior %.4f", posterior2)
	if posterior2 < 0.55 || posterior2 > 0.80 {
		t.Errorf("expected moderately high posterior, got %.4f", posterior2)
	}

	// Conflicting evidence: one for, one strongly against
	posterior3, err := BayesianCompose(0.5, []Evidence{
		{SourceID: "for", Confidence: 0.9, LikelihoodRatio: 8.0},
		{SourceID: "against", Confidence: 0.9, LikelihoodRatio: 0.1},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Prior 0.5 + conflicting evidence → posterior %.4f", posterior3)
	// Should be somewhere in the middle, pulled toward neutral
}

func TestBayesianConfidenceWeighting(t *testing.T) {
	// High LR but low confidence source should contribute less
	high_conf, _ := BayesianCompose(0.1, []Evidence{
		{SourceID: "certain", Confidence: 0.99, LikelihoodRatio: 20.0},
	})
	low_conf, _ := BayesianCompose(0.1, []Evidence{
		{SourceID: "uncertain", Confidence: 0.20, LikelihoodRatio: 20.0},
	})
	t.Logf("LR=20: high-conf source → %.4f, low-conf source → %.4f", high_conf, low_conf)
	if high_conf <= low_conf {
		t.Error("high-confidence source should yield higher posterior than low-confidence with same LR")
	}
}

func TestDempsterShaferCompose(t *testing.T) {
	// Two agreeing sources
	m1 := DempsterShaferMass{SourceID: "s1", MassTrue: 0.7, MassFalse: 0.1, MassUnknown: 0.2}
	m2 := DempsterShaferMass{SourceID: "s2", MassTrue: 0.6, MassFalse: 0.2, MassUnknown: 0.2}

	bel, pls, K, err := DempsterShaferCompose(m1, m2)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Agreeing sources: belief=%.4f plausibility=%.4f conflict=%.4f", bel, pls, K)
	if K > 0.3 {
		t.Error("agreeing sources should have low conflict")
	}
	if bel < 0.6 {
		t.Errorf("agreeing sources should yield high belief, got %.4f", bel)
	}

	// Contradicting sources: one certain true, one certain false
	m3 := DempsterShaferMass{SourceID: "for", MassTrue: 0.9, MassFalse: 0.05, MassUnknown: 0.05}
	m4 := DempsterShaferMass{SourceID: "against", MassTrue: 0.05, MassFalse: 0.9, MassUnknown: 0.05}

	bel2, pls2, K2, err2 := DempsterShaferCompose(m3, m4)
	if err2 != nil {
		t.Fatal(err2)
	}
	t.Logf("Contradicting sources: belief=%.4f plausibility=%.4f conflict=%.4f", bel2, pls2, K2)
	if K2 < 0.5 {
		t.Error("contradicting sources should have high conflict")
	}

	// Total conflict
	m5 := DempsterShaferMass{SourceID: "certain-true", MassTrue: 1.0, MassFalse: 0, MassUnknown: 0}
	m6 := DempsterShaferMass{SourceID: "certain-false", MassTrue: 0, MassFalse: 1.0, MassUnknown: 0}
	_, _, _, err3 := DempsterShaferCompose(m5, m6)
	if err3 == nil {
		t.Error("total conflict should return an error")
	}
	t.Logf("Total conflict error: %v", err3)
}

func TestValidateConfidence(t *testing.T) {
	// Well-calibrated asserter
	cb, err := ValidateConfidence(0.72, 0.3, []Evidence{
		{SourceID: "bp-high", Confidence: 0.85, LikelihoodRatio: 5.0},
		{SourceID: "family-history", Confidence: 0.70, LikelihoodRatio: 3.0},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(FormatComposed(cb))
	if cb.OverconfidenceWarn || cb.UnderconfidenceWarn {
		t.Logf("calibration warning: discrepancy=%.3f", cb.Discrepancy)
	}

	// Overconfident asserter: claims 0.95 but evidence only supports ~0.7
	cb2, err := ValidateConfidence(0.95, 0.3, []Evidence{
		{SourceID: "weak-evidence", Confidence: 0.6, LikelihoodRatio: 2.0},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(FormatComposed(cb2))
	if !cb2.OverconfidenceWarn {
		t.Errorf("expected overconfidence warning, computed=%.3f declared=%.3f",
			cb2.ComputedConfidence, cb2.DeclaredConfidence)
	}

	// Underconfident: hedges more than evidence requires
	cb3, err := ValidateConfidence(0.30, 0.3, []Evidence{
		{SourceID: "strong-evidence", Confidence: 0.95, LikelihoodRatio: 15.0},
		{SourceID: "corroboration", Confidence: 0.88, LikelihoodRatio: 8.0},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(FormatComposed(cb3))
	if !cb3.UnderconfidenceWarn {
		t.Errorf("expected underconfidence warning, computed=%.3f declared=%.3f",
			cb3.ComputedConfidence, cb3.DeclaredConfidence)
	}
}

func TestBayesianVsDempsterShafer(t *testing.T) {
	// Compare how the two frameworks handle the same evidence scenario
	// Scenario: is there life on Europa?
	// Prior: low (0.1)
	// Evidence 1: subsurface ocean confirmed (LR ≈ 3)
	// Evidence 2: heat source detected (LR ≈ 4)
	// Evidence 3: complex organic molecules detected (LR ≈ 8)

	posterior, _ := BayesianCompose(0.1, []Evidence{
		{SourceID: "ocean", Confidence: 0.95, LikelihoodRatio: 3.0},
		{SourceID: "heat", Confidence: 0.85, LikelihoodRatio: 4.0},
		{SourceID: "organics", Confidence: 0.70, LikelihoodRatio: 8.0},
	})

	// DS: convert the same evidence to mass functions
	// Ocean: mostly supports (mass 0.4 true), some uncertainty
	m1 := DempsterShaferMass{SourceID: "ocean", MassTrue: 0.40, MassFalse: 0.05, MassUnknown: 0.55}
	m2 := DempsterShaferMass{SourceID: "heat", MassTrue: 0.50, MassFalse: 0.05, MassUnknown: 0.45}
	bel12, pls12, K12, _ := DempsterShaferCompose(m1, m2)
	m3 := DempsterShaferMass{SourceID: "organics-combined", MassTrue: bel12, MassFalse: 1 - pls12, MassUnknown: pls12 - bel12}
	m4 := DempsterShaferMass{SourceID: "organics", MassTrue: 0.65, MassFalse: 0.10, MassUnknown: 0.25}
	bel, pls, K, err := DempsterShaferCompose(m3, m4)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Europa life:")
	t.Logf("  Bayesian posterior: %.4f", posterior)
	t.Logf("  DS belief [%.4f, plausibility %.4f] conflict=%.4f (at m1*m2 stage: K=%.4f)", bel, pls, K, K12)
	t.Logf("  Key difference: DS expresses uncertainty as an interval [%.4f, %.4f]; Bayes gives a point estimate", bel, pls)

	// The DS interval [belief, plausibility] is typically wider than the Bayesian point estimate
	// because DS preserves ignorance explicitly
	intervalWidth := pls - bel
	t.Logf("  DS uncertainty interval width: %.4f", intervalWidth)

	// Bayesian should be within the DS interval
	if posterior < bel-0.1 || posterior > pls+0.1 {
		t.Logf("Bayesian estimate %.4f lies outside DS interval [%.4f, %.4f] — interesting divergence", posterior, bel, pls)
	}

	// Sanity checks
	if posterior < 0 || posterior > 1 {
		t.Errorf("invalid Bayesian posterior: %.4f", posterior)
	}
	if math.IsNaN(bel) || math.IsNaN(pls) {
		t.Error("DS produced NaN")
	}
}

func TestCorrelatedEvidence(t *testing.T) {
	// Hard problem: zombie argument, knowledge argument, conceivability argument
	// These three are correlated — they all rely on the same intuition
	// that there's an explanatory gap between physical description and experience.
	prior := 0.50

	// Naive: treat as independent
	naive, _ := BayesianCompose(prior, []Evidence{
		{SourceID: "zombie", Confidence: 0.65, LikelihoodRatio: 4.5},
		{SourceID: "knowledge", Confidence: 0.70, LikelihoodRatio: 3.5},
		{SourceID: "conceivability", Confidence: 0.60, LikelihoodRatio: 3.0},
	})

	// Correlated: zombie and knowledge share ~0.7 of their evidential weight
	// (both rely on the same modal intuition), conceivability is also correlated
	correlated := []CorrelatedEvidence{
		{Evidence: Evidence{SourceID: "zombie", Confidence: 0.65, LikelihoodRatio: 4.5},
			CorrelationWith: map[string]float64{"knowledge": 0.70, "conceivability": 0.60}},
		{Evidence: Evidence{SourceID: "knowledge", Confidence: 0.70, LikelihoodRatio: 3.5},
			CorrelationWith: map[string]float64{"zombie": 0.70, "conceivability": 0.50}},
		{Evidence: Evidence{SourceID: "conceivability", Confidence: 0.60, LikelihoodRatio: 3.0},
			CorrelationWith: map[string]float64{"zombie": 0.60, "knowledge": 0.50}},
	}
	adjusted, _ := BayesianComposeCorrelated(prior, correlated)

	report, _ := CompareNaiveVsCorrelated(prior, correlated)

	t.Logf("Hard problem evidence — naive vs correlated:")
	t.Logf("  Naive (independent assumption): %.4f", naive)
	t.Logf("  Correlated (adjusted):          %.4f", adjusted)
	t.Logf("  Overcounting reduced:           %.4f (naive was inflated by this much)", report.OvercountingReduced)
	t.Logf("  Source weights after adjustment:")
	for id, w := range report.SourceWeights {
		t.Logf("    %-20s weight=%.3f", id, w)
	}

	// The correlated posterior should be lower than the naive one
	// because correlated sources count less than independent ones
	if adjusted >= naive {
		t.Errorf("correlated posterior (%.4f) should be lower than naive (%.4f) for positively correlated support", adjusted, naive)
	}

	// The reduction should be meaningful (>5 percentage points)
	if report.OvercountingReduced < 0.05 {
		t.Errorf("expected meaningful reduction from correlation adjustment, got %.4f", report.OvercountingReduced)
	}

	t.Logf("")
	t.Logf("Implication: the philosophical case for the hard problem is weaker than")
	t.Logf("naive Bayesian combination suggests, because the main arguments share")
	t.Logf("the same modal intuition as their epistemic root.")
}

func TestUncorrelatedBaselineUnchanged(t *testing.T) {
	// With zero correlation, BayesianComposeCorrelated should match BayesianCompose
	prior := 0.30
	ev := []Evidence{
		{SourceID: "e1", Confidence: 0.80, LikelihoodRatio: 3.0},
		{SourceID: "e2", Confidence: 0.75, LikelihoodRatio: 5.0},
	}
	naive, _ := BayesianCompose(prior, ev)

	correlated := []CorrelatedEvidence{
		{Evidence: ev[0], CorrelationWith: map[string]float64{}},
		{Evidence: ev[1], CorrelationWith: map[string]float64{}},
	}
	adjusted, _ := BayesianComposeCorrelated(prior, correlated)

	if math.Abs(naive-adjusted) > 0.001 {
		t.Errorf("uncorrelated evidence should give same result: naive=%.4f adjusted=%.4f", naive, adjusted)
	}
}

func TestBayesianComposedRandomStress(t *testing.T) {
	// Generate 500 random belief chains and verify posterior always in [0,1]
	// Uses a deterministic pseudo-random sequence for reproducibility
	seed := uint64(20260819)
	lcg := func() float64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		return float64(seed>>11) / float64(1<<53)
	}

	failures := 0
	for i := 0; i < 500; i++ {
		prior := 0.01 + lcg()*0.98 // (0.01, 0.99)
		nEvidence := 1 + int(lcg()*9) // 1..10

		evidence := make([]Evidence, nEvidence)
		for j := range evidence {
			// LR in range (0.01, 20) — mix of supporting and undermining
			lr := 0.01 + lcg()*19.99
			conf := 0.1 + lcg()*0.89
			evidence[j] = Evidence{
				SourceID:        fmt.Sprintf("e%d", j),
				LikelihoodRatio: lr,
				Confidence:      conf,
			}
		}

		posterior, err := BayesianCompose(prior, evidence)
		if err != nil {
			failures++
			continue
		}
		if posterior < 0 || posterior > 1 {
			t.Errorf("iteration %d: posterior %.6f out of [0,1] (prior=%.3f, n=%d)",
				i, posterior, prior, nEvidence)
			failures++
		}
	}
	t.Logf("Random stress test: 500 iterations, %d failures", failures)
	if failures > 0 {
		t.Errorf("%d failures in random stress test", failures)
	}
}

func TestCorrelatedComposedRandomStress(t *testing.T) {
	// Same but with BayesianComposeCorrelated
	seed := uint64(20260820)
	lcg := func() float64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		return float64(seed>>11) / float64(1<<53)
	}

	failures := 0
	for i := 0; i < 500; i++ {
		prior := 0.01 + lcg()*0.98
		nEvidence := 2 + int(lcg()*8) // 2..10

		evidence := make([]CorrelatedEvidence, nEvidence)
		for j := range evidence {
			lr := 0.01 + lcg()*19.99
			conf := 0.1 + lcg()*0.89
			corrWith := make(map[string]float64)
			// Add some random correlations with other sources
			if j > 0 && lcg() > 0.5 {
				corrWith[fmt.Sprintf("e%d", j-1)] = lcg() * 0.9
			}
			evidence[j] = CorrelatedEvidence{
				Evidence:        Evidence{SourceID: fmt.Sprintf("e%d", j), LikelihoodRatio: lr, Confidence: conf},
				CorrelationWith: corrWith,
			}
		}

		posterior, err := BayesianComposeCorrelated(prior, evidence)
		if err != nil {
			failures++
			continue
		}
		if posterior < 0 || posterior > 1 {
			t.Errorf("iteration %d: correlated posterior %.6f out of [0,1]", i, posterior)
			failures++
		}
	}
	t.Logf("Correlated random stress: 500 iterations, %d failures", failures)
	if failures > 0 {
		t.Errorf("%d failures in correlated random stress test", failures)
	}
}

func BenchmarkBayesianCompose(b *testing.B) {
	evidence1 := []Evidence{{SourceID: "e1", Confidence: 0.85, LikelihoodRatio: 5.0}}
	evidence5 := []Evidence{
		{SourceID: "e1", Confidence: 0.85, LikelihoodRatio: 5.0},
		{SourceID: "e2", Confidence: 0.75, LikelihoodRatio: 3.0},
		{SourceID: "e3", Confidence: 0.60, LikelihoodRatio: 0.3},
		{SourceID: "e4", Confidence: 0.90, LikelihoodRatio: 8.0},
		{SourceID: "e5", Confidence: 0.70, LikelihoodRatio: 2.0},
	}
	evidence10 := make([]Evidence, 10)
	for i := range evidence10 {
		evidence10[i] = Evidence{SourceID: fmt.Sprintf("e%d", i), Confidence: 0.75, LikelihoodRatio: float64(i+1)}
	}

	b.Run("n=1", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			BayesianCompose(0.3, evidence1)
		}
	})
	b.Run("n=5", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			BayesianCompose(0.3, evidence5)
		}
	})
	b.Run("n=10", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			BayesianCompose(0.3, evidence10)
		}
	})
}

func BenchmarkBayesianComposeCorrelated(b *testing.B) {
	evidence5 := []CorrelatedEvidence{
		{Evidence: Evidence{SourceID: "e1", Confidence: 0.85, LikelihoodRatio: 5.0},
			CorrelationWith: map[string]float64{"e2": 0.5}},
		{Evidence: Evidence{SourceID: "e2", Confidence: 0.75, LikelihoodRatio: 3.0},
			CorrelationWith: map[string]float64{"e1": 0.5, "e3": 0.3}},
		{Evidence: Evidence{SourceID: "e3", Confidence: 0.60, LikelihoodRatio: 0.3},
			CorrelationWith: map[string]float64{"e2": 0.3}},
		{Evidence: Evidence{SourceID: "e4", Confidence: 0.90, LikelihoodRatio: 8.0},
			CorrelationWith: map[string]float64{}},
		{Evidence: Evidence{SourceID: "e5", Confidence: 0.70, LikelihoodRatio: 2.0},
			CorrelationWith: map[string]float64{}},
	}
	b.Run("n=5_correlated", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			BayesianComposeCorrelated(0.3, evidence5)
		}
	})
}
