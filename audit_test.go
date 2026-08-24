package lumen

import (
	"math"
	"strings"
	"testing"
)

func TestSensitivityAnalysis(t *testing.T) {
	// hard-problem-real evidence from the philosophy analysis
	prior := 0.50
	evidence := []Evidence{
		{SourceID: "zombie-argument", Confidence: 0.65, LikelihoodRatio: 4.5},
		{SourceID: "knowledge-argument", Confidence: 0.70, LikelihoodRatio: 3.5},
		{SourceID: "illusionism-dennett", Confidence: 0.60, LikelihoodRatio: 0.35}, // counter-evidence
		{SourceID: "anthropic-circuits-2025", Confidence: 0.90, LikelihoodRatio: 2.0},
	}

	full, err := BayesianCompose(prior, evidence)
	if err != nil {
		t.Fatal(err)
	}

	sens, err := SensitivityAnalysis(prior, evidence, full)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("hard-problem-real sensitivity (prior=%.2f, full=%.3f):", prior, full)
	for _, s := range sens.Sources {
		t.Logf("  rank %d: %+.3f  %s (LR=%.2f)", s.Rank, s.MarginalContribution, s.SourceID, s.LikelihoodRatio)
	}

	// zombie-argument (LR=4.5) should be the top positive driver
	if sens.Sources[0].SourceID != "zombie-argument" {
		t.Errorf("expected zombie-argument as top driver, got %s", sens.Sources[0].SourceID)
	}
	if sens.Sources[0].MarginalContribution <= 0 {
		t.Error("zombie-argument should contribute positively")
	}

	// illusionism-dennett (LR=0.35) should be the negative driver
	var illusionism SourceContribution
	for _, s := range sens.Sources {
		if s.SourceID == "illusionism-dennett" {
			illusionism = s
		}
	}
	if illusionism.MarginalContribution >= 0 {
		t.Errorf("illusionism-dennett (LR=0.35) should contribute negatively, got %+.3f", illusionism.MarginalContribution)
	}
}

func TestSensitivityIIT(t *testing.T) {
	// iit-viable evidence — confirm cogitate-2023 is dominant negative driver
	prior := 0.25
	evidence := []Evidence{
		{SourceID: "iit-tononi", Confidence: 0.70, LikelihoodRatio: 3.0},
		{SourceID: "ncc-posterior", Confidence: 0.85, LikelihoodRatio: 1.8},
		{SourceID: "cogitate-2023", Confidence: 0.90, LikelihoodRatio: 0.25},
		{SourceID: "iit-letter-2023", Confidence: 0.75, LikelihoodRatio: 0.30},
	}

	full, _ := BayesianCompose(prior, evidence)
	sens, err := SensitivityAnalysis(prior, evidence, full)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("iit-viable sensitivity (prior=%.2f, full=%.3f):", prior, full)
	for _, s := range sens.Sources {
		t.Logf("  rank %d: %+.3f  %s", s.Rank, s.MarginalContribution, s.SourceID)
	}

	// cogitate-2023 (LR=0.25) should be the top negative driver by absolute magnitude
	topDriver := sens.Sources[0]
	if topDriver.MarginalContribution >= 0 {
		t.Errorf("top driver for iit should be negative (counter-evidence), got %+.3f for %s",
			topDriver.MarginalContribution, topDriver.SourceID)
	}
	// It should be either cogitate or iit-letter (both negative)
	if topDriver.SourceID != "cogitate-2023" && topDriver.SourceID != "iit-letter-2023" {
		t.Errorf("expected cogitate-2023 or iit-letter-2023 as top driver, got %s", topDriver.SourceID)
	}
}

func TestAuditReport(t *testing.T) {
	priors := map[string]float64{
		"belief-a": 0.30,
		"belief-b": 0.50,
		"belief-c": 0.20,
	}
	evidenceMap := map[string][]Evidence{
		"belief-a": {
			{SourceID: "e1", Confidence: 0.85, LikelihoodRatio: 5.0},
			{SourceID: "e2", Confidence: 0.70, LikelihoodRatio: 3.0},
		},
		"belief-b": {
			{SourceID: "e3", Confidence: 0.60, LikelihoodRatio: 2.0},
		},
		"belief-c": {
			{SourceID: "e4", Confidence: 0.95, LikelihoodRatio: 15.0},
			{SourceID: "e5", Confidence: 0.88, LikelihoodRatio: 8.0},
		},
	}

	// Build composed beliefs
	composed := []*ComposedBelief{}
	for id, ev := range evidenceMap {
		prior := priors[id]
		cb, err := ValidateConfidence(0.50, prior, ev) // declare 0.50 for all to force various gaps
		if err != nil {
			t.Fatal(err)
		}
		cb.ID = id
		cb.Content = "Test belief " + id
		composed = append(composed, cb)
	}

	report := BuildAuditReport(composed, priors, evidenceMap)
	t.Log("\n" + FormatAuditReport(report))

	if report.TotalBeliefs != 3 {
		t.Errorf("expected 3 beliefs, got %d", report.TotalBeliefs)
	}
	if report.MeanDiscrepancy <= 0 {
		t.Error("expected non-zero mean discrepancy")
	}
	if report.WorstBeliefID == "" {
		t.Error("expected worst belief identified")
	}
	if !strings.Contains(FormatAuditReport(report), "Sensitivity") {
		t.Error("expected sensitivity analysis in report")
	}
}

func TestSensitivitySingleSource(t *testing.T) {
	// Single source: removing it leaves only the prior
	prior := 0.30
	evidence := []Evidence{
		{SourceID: "only-source", Confidence: 0.90, LikelihoodRatio: 8.0},
	}
	full, _ := BayesianCompose(prior, evidence)
	sens, err := SensitivityAnalysis(prior, evidence, full)
	if err != nil {
		t.Fatal(err)
	}

	// Without the only source, posterior = prior
	if math.Abs(sens.Sources[0].PosteriorWithout-prior) > 0.001 {
		t.Errorf("removing only source should give prior %.3f, got %.3f",
			prior, sens.Sources[0].PosteriorWithout)
	}
	t.Logf("Single source: full=%.3f without=%.3f contribution=%+.3f",
		full, sens.Sources[0].PosteriorWithout, sens.Sources[0].MarginalContribution)
}
