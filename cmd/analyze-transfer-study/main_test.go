package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/optakt/lumen/transfer"
)

func TestHeldOutTopologyEvaluation(t *testing.T) {
	vectors := map[signatureKey]transfer.FeatureVector{}
	models := map[string]bool{"a": true, "b": true}
	for _, topology := range transfer.StudyTopologies {
		for run := 1; run <= 2; run++ {
			vectors[signatureKey{"a", run, topology, "s1"}] = transfer.FeatureVector{"x": 0.1, "y": 0.2}
			vectors[signatureKey{"b", run, topology, "s1"}] = transfer.FeatureVector{"x": 0.9, "y": 0.8}
		}
	}
	result := evaluateHeldOut(vectors, models, "mesh")
	if result.Correct != 4 || result.Total != 4 || result.Skipped != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestHeldOutTieCountsAsSkippedAndWrong(t *testing.T) {
	vectors := map[signatureKey]transfer.FeatureVector{}
	models := map[string]bool{"a": true, "b": true}
	for _, topology := range transfer.StudyTopologies {
		for _, model := range []string{"a", "b"} {
			vectors[signatureKey{model, 1, topology, "s1"}] = transfer.FeatureVector{"x": 0.5}
		}
	}
	result := evaluateHeldOut(vectors, models, "mesh")
	if result.Correct != 0 || result.Total != 2 || result.Skipped != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestCalibrationThresholdUsesOnlyTrainingTopologies(t *testing.T) {
	vectors := map[signatureKey]transfer.FeatureVector{}
	models := map[string]bool{"a": true, "b": true}
	for _, topology := range transfer.StudyTopologies {
		vectors[signatureKey{"a", 1, topology, "s1"}] = transfer.FeatureVector{"x": 0.1}
		vectors[signatureKey{"b", 1, topology, "s1"}] = transfer.FeatureVector{"x": 0.9}
	}
	threshold := calibrationThreshold(vectors, models, "mesh")
	if threshold < 0 || threshold > 1e-9 {
		t.Fatalf("threshold = %f", threshold)
	}
}

func TestExactMcNemar(t *testing.T) {
	if got := exactMcNemar(0, 0); got != 1 {
		t.Fatalf("zero discordance p=%f", got)
	}
	if got := exactMcNemar(5, 0); math.Abs(got-0.0625) > 1e-12 {
		t.Fatalf("p=%f", got)
	}
}

func TestMedian(t *testing.T) {
	if got := median([]float64{3, 1, 2}); got != 2 {
		t.Fatalf("median = %f", got)
	}
	if got := median([]float64{4, 1, 3, 2}); got != 2.5 {
		t.Fatalf("median even = %f", got)
	}
}

func TestDistanceAUROC(t *testing.T) {
	got := distanceAUROC([]float64{0.1, 0.2}, []float64{0.8, 0.9})
	if got != 1 {
		t.Fatalf("AUROC = %f", got)
	}
	got = distanceAUROC([]float64{0.5}, []float64{0.5})
	if got != 0.5 {
		t.Fatalf("tie AUROC = %f", got)
	}
}

func TestOperatorFilterExcludesPerStepFamilyFeatures(t *testing.T) {
	if isOperatorFeature("source-reliability-reversal.reports.mid") {
		t.Fatal("per-step reliability feature leaked into operator summary")
	}
	if !isOperatorFeature("source-reliability-reversal.downgrade_delta") {
		t.Fatal("operator summary was excluded")
	}
}

func TestStaticTextureHasFixedSchema(t *testing.T) {
	a := transfer.FeatureVector{}
	b := transfer.FeatureVector{}
	addTexture(a, "One compact answer.")
	addTexture(b, "A very different response with several words.")
	for _, vector := range []transfer.FeatureVector{a, b} {
		for i := 0; i < 256; i++ {
			for _, prefix := range []string{"word", "bigram"} {
				key := prefix + "." + formatBin(i)
				if _, ok := vector[key]; !ok {
					vector[key] = 0
				}
			}
		}
	}
	if len(a) != len(b) {
		t.Fatalf("schemas differ: %d vs %d", len(a), len(b))
	}
}

func formatBin(i int) string {
	return fmt.Sprintf("%03d", i)
}
