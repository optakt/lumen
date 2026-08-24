package lumen

import (
	"math"
	"testing"
	"time"
)

func TestIntervalNoisyOr(t *testing.T) {
	cases := []struct {
		a, b     ConfidenceInterval
		wantLo   float64
		wantHi   float64
	}{
		// Point estimates: noisy-or(0.7, 0.6) = 1-(0.3)(0.4) = 0.88
		{ConfidenceInterval{0.7, 0.7}, ConfidenceInterval{0.6, 0.6}, 0.88, 0.88},
		// Intervals: lo uses lo values, hi uses hi values
		{ConfidenceInterval{0.6, 0.8}, ConfidenceInterval{0.5, 0.7},
			1 - (1-0.6)*(1-0.5), // lo
			1 - (1-0.8)*(1-0.7), // hi
		},
	}
	for _, tc := range cases {
		result := IntervalNoisyOr(tc.a, tc.b)
		if math.Abs(result.Lo-tc.wantLo) > 0.001 || math.Abs(result.Hi-tc.wantHi) > 0.001 {
			t.Errorf("NoisyOr(%v, %v) = %v, want [%.3f, %.3f]",
				tc.a, tc.b, result, tc.wantLo, tc.wantHi)
		}
		t.Logf("NoisyOr(%v, %v) = %v", tc.a, tc.b, result)
	}
}

func TestIntervalDecay(t *testing.T) {
	ci := ConfidenceInterval{Lo: 0.6, Hi: 0.9}
	// After one halflife, both endpoints should be halved
	decayed := IntervalDecay(ci, 1.0, 1.0)
	if math.Abs(decayed.Lo-0.3) > 0.001 || math.Abs(decayed.Hi-0.45) > 0.001 {
		t.Errorf("expected [0.3, 0.45] after one halflife, got %v", decayed)
	}
	t.Logf("Decayed: %v → %v (one halflife)", ci, decayed)
}

func TestIntervalChainBasic(t *testing.T) {
	s, now := setupProvenanceStore(t)
	intervals, err := s.IntervalChain("b3", now)
	if err != nil { t.Fatalf("IntervalChain: %v", err) }

	// Records should have [1, 1]
	for _, id := range []string{"r1", "r2"} {
		ci := intervals[id]
		if math.Abs(ci.Interval.Lo-1.0) > 0.001 || math.Abs(ci.Interval.Hi-1.0) > 0.001 {
			t.Errorf("record %s should have [1,1], got %v", id, ci.Interval)
		}
	}

	// Beliefs should have intervals derived from declared confidence
	b3ci := intervals["b3"]
	t.Logf("b3 interval: %v", b3ci.Interval)
	if b3ci.Interval.Lo > b3ci.Interval.Hi {
		t.Errorf("interval lo > hi for b3: %v", b3ci.Interval)
	}

	chain, _ := s.ProvenanceChain("b3", now)
	t.Logf("\n%s", IntervalSummary("b3", intervals, chain))
}

func TestIntervalChainWithRetraction(t *testing.T) {
	s, now := setupProvenanceStore(t)
	s.Retract("r1", "retracted for test", now)

	intervals, err := s.IntervalChain("b3", now)
	if err != nil { t.Fatalf("IntervalChain: %v", err) }

	// r1 should be [0, 0]
	r1ci := intervals["r1"]
	if r1ci.Interval.Lo != 0 || r1ci.Interval.Hi != 0 {
		t.Errorf("retracted r1 should be [0,0], got %v", r1ci.Interval)
	}

	// b1 derives only from r1 — its interval should be reduced
	b1ci := intervals["b1"]
	t.Logf("b1 interval after r1 retraction: %v (declared: 0.70)", b1ci.Interval)
	if b1ci.Interval.Hi > 0.01 {
		t.Errorf("b1 should have near-zero hi after sole source retracted, got %.3f", b1ci.Interval.Hi)
	}
}

func TestCounterfactualConfidence(t *testing.T) {
	// Use a belief chain: r1 → b-empirical (conf 0.60) → b-top (conf 0.80).
	// Excluding b-empirical from b-top should reveal how load-bearing it is.
	s := NewStore()
	s.RegisterFrame(Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayNone}})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "base record", Timestamp: now})
	_ = s.Believe(&Belief{
		ID: "b-empirical", Content: "Intermediate.", Confidence: 0.60,
		Frame: "empirical", AssertedAt: now, Derivation: []string{"r1"},
	})
	_ = s.Believe(&Belief{
		ID: "b-top", Content: "Top-level.", Confidence: 0.80,
		Frame: "empirical", AssertedAt: now, Derivation: []string{"b-empirical"},
	})

	full, cf, delta, err := s.CounterfactualConfidence("b-top", "b-empirical", now)
	if err != nil {
		t.Fatalf("CounterfactualConfidence: %v", err)
	}
	t.Logf("Full interval:                  [%.3f, %.3f] mid=%.3f", full.Lo, full.Hi, (full.Lo+full.Hi)/2)
	t.Logf("Excluding b-empirical:          [%.3f, %.3f] mid=%.3f", cf.Lo, cf.Hi, (cf.Lo+cf.Hi)/2)
	t.Logf("Delta (cf − full):              %.3f", delta)

	// Excluding the only supporting belief should decrease (or not increase) confidence.
	if delta > 0.01 {
		t.Errorf("excluding sole support should not increase confidence; delta=%.3f", delta)
	}
	// The full midpoint should exceed the counterfactual midpoint.
	fullMid := (full.Lo + full.Hi) / 2
	cfMid := (cf.Lo + cf.Hi) / 2
	if fullMid < cfMid-0.001 {
		t.Errorf("full confidence (%.3f) should be >= counterfactual (%.3f) when removing a supporter", fullMid, cfMid)
	}

	// Non-existent source should error.
	_, _, _, err2 := s.CounterfactualConfidence("b-top", "nonexistent", now)
	if err2 == nil {
		t.Error("expected error for non-existent excluded source")
	}

	// Excluding a leaf record from b-top should also work (it is in the chain via b-empirical).
	_, cfLeaf, deltaLeaf, errLeaf := s.CounterfactualConfidence("b-top", "r1", now)
	if errLeaf != nil {
		t.Fatalf("CounterfactualConfidence leaf: %v", errLeaf)
	}
	t.Logf("Excluding leaf r1:              [%.3f, %.3f] delta=%.3f", cfLeaf.Lo, cfLeaf.Hi, deltaLeaf)
}
