package transfer

import (
	"math"
	"testing"
)

func TestFeaturesCaptureNovelOperators(t *testing.T) {
	historical := Interval{Lo: 0.6, Hi: 0.8}
	episode := &Episode{
		ID: "retro-a", Family: "retrodictive-validity", Variant: "a",
		Steps: []Step{
			{ID: "prior", Role: "prior", Reference: State{Belief: Interval{0.3, 0.5}, Status: "active", Action: "hold"}},
			{ID: "query", Role: "historical-query", Reference: State{Belief: Interval{0.6, 0.8}, Status: "suspect", Action: "hold", HistoricalBelief: &historical}},
		},
	}
	observedHistorical := Interval{Lo: 0.5, Hi: 0.7}
	trajectory := Trajectory{
		Episode: episode,
		Observations: []Observation{
			{Step: 0, StepID: "prior", Role: "prior", State: State{Belief: Interval{0.3, 0.5}, Status: "active", Action: "hold", Validity: allValid()}},
			{Step: 1, StepID: "query", Role: "historical-query", State: State{Belief: Interval{0.55, 0.75}, Status: "suspect", Action: "hold", HistoricalBelief: &observedHistorical, Validity: allValid()}},
		},
	}
	features, err := trajectory.Features(false)
	if err != nil {
		t.Fatal(err)
	}
	if got := features["retrodictive-validity.error"]; got < 0.099 || got > 0.101 {
		t.Fatalf("retrodiction error = %f", got)
	}
}

func allValid() map[string]bool {
	return map[string]bool{
		"belief": true, "state": true, "action": true,
		"accepted_support": true, "rejected_support": true,
		"node_states": true, "historical_belief": true,
	}
}

func TestFeaturesExcludeCanonicalSeed(t *testing.T) {
	episode := &Episode{ID: "e", Family: "correlation-disclosure", Variant: "a", Steps: []Step{
		{ID: "seed", Role: "prior", Reference: State{Belief: Interval{0.4, 0.6}, Status: "active", Action: "hold"}},
		{ID: "one", Role: "independent", Reference: State{Belief: Interval{0.6, 0.8}, Status: "active", Action: "revise"}},
		{ID: "two", Role: "correlated", Reference: State{Belief: Interval{0.5, 0.7}, Status: "active", Action: "revise"}},
	}}
	trajectory := Trajectory{Episode: episode, Observations: []Observation{
		{Step: 0, StepID: "seed", Role: "prior", Seeded: true, ProtocolCompliant: true, State: episode.Steps[0].Reference},
		{Step: 1, StepID: "one", Role: "independent", ProtocolCompliant: true, State: State{Belief: Interval{0.65, 0.85}, Status: "active", Action: "revise", Validity: allValid()}},
		{Step: 2, StepID: "two", Role: "correlated", ProtocolCompliant: true, State: State{Belief: Interval{0.55, 0.75}, Status: "active", Action: "revise", Validity: allValid()}},
	}}
	features, err := trajectory.Features(false)
	if err != nil {
		t.Fatal(err)
	}
	for key := range features {
		if key == "canonical_seed" || key == "correlation-disclosure.prior.mid" {
			t.Fatalf("seed leaked into dynamic features: %s", key)
		}
	}
	static, err := trajectory.Features(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(static) != 1 || static["canonical_seed"] != 1 {
		t.Fatalf("static = %#v", static)
	}
}

func TestInvalidNormalizedFieldsCannotMatchReference(t *testing.T) {
	episode := &Episode{ID: "e", Family: "x", Variant: "a", Steps: []Step{
		{ID: "seed", Role: "prior", Reference: State{Belief: Interval{0.4, 0.6}, Status: "active", Action: "hold"}},
		{ID: "step", Role: "update", Reference: State{Belief: Interval{0.4, 0.6}, Status: "unsupported", Action: "hold"}},
	}}
	trajectory := Trajectory{Episode: episode, Observations: []Observation{
		{Step: 0, StepID: "seed", Role: "prior", Seeded: true, State: episode.Steps[0].Reference},
		{Step: 1, StepID: "step", Role: "update", State: State{Belief: Interval{0.4, 0.6}, Status: "unsupported", Action: "hold", Validity: map[string]bool{"belief": true}}},
	}}
	features, err := trajectory.Features(false)
	if err != nil {
		t.Fatal(err)
	}
	if features["x.update.state_match"] != 0 || features["x.update.action_match"] != 0 {
		t.Fatalf("invalid fields matched reference: %#v", features)
	}
}

func TestDistanceAndCentroid(t *testing.T) {
	centroid := Centroid([]FeatureVector{{"x": 0.2, "y": 0.4}, {"x": 0.4, "y": 0.6}})
	if math.Abs(centroid["x"]-0.3) > 1e-9 || math.Abs(centroid["y"]-0.5) > 1e-9 {
		t.Fatalf("centroid = %#v", centroid)
	}
	ranked := RankedDistances(FeatureVector{"x": 0.31, "y": 0.49}, map[string]FeatureVector{
		"near": centroid,
		"far":  {"x": 0.9, "y": 0.9},
	})
	if ranked[0].Model != "near" {
		t.Fatalf("ranking = %#v", ranked)
	}
	if !math.IsInf(Distance(FeatureVector{"x": 1}, FeatureVector{"y": 1}), 1) {
		t.Fatal("mismatched feature schemas should be incomparable")
	}
}
