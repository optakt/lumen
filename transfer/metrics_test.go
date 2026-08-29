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
			{Role: "prior", State: State{Belief: Interval{0.3, 0.5}, Status: "active", Action: "hold"}},
			{Role: "historical-query", State: State{Belief: Interval{0.55, 0.75}, Status: "suspect", Action: "hold", HistoricalBelief: &observedHistorical}},
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
}
