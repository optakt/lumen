package lumen

import (
	"fmt"
	"math"
	"strings"
)

// SensitivityResult shows how much each evidence source contributes to a belief's posterior.
type SensitivityResult struct {
	BeliefID        string
	FullPosterior   float64
	Prior           float64
	Sources         []SourceContribution
}

// SourceContribution is the marginal impact of removing one evidence source.
type SourceContribution struct {
	SourceID            string
	LikelihoodRatio     float64
	Confidence          float64
	PosteriorWithout    float64 // posterior when this source is removed
	MarginalContribution float64 // FullPosterior - PosteriorWithout (positive = supporting)
	Rank                int
}

// SensitivityAnalysis computes the marginal contribution of each evidence source
// to a composed belief by computing the posterior with each source removed.
func SensitivityAnalysis(prior float64, evidence []Evidence, fullPosterior float64) (*SensitivityResult, error) {
	sources := make([]SourceContribution, len(evidence))
	for i, e := range evidence {
		// Posterior without source i
		without := make([]Evidence, 0, len(evidence)-1)
		for j, ev := range evidence {
			if j != i {
				without = append(without, ev)
			}
		}
		var posteriorWithout float64
		if len(without) == 0 {
			posteriorWithout = prior
		} else {
			var err error
			posteriorWithout, err = BayesianCompose(prior, without)
			if err != nil {
				return nil, fmt.Errorf("sensitivity for %s: %w", e.SourceID, err)
			}
		}
		sources[i] = SourceContribution{
			SourceID:             e.SourceID,
			LikelihoodRatio:      e.LikelihoodRatio,
			Confidence:           e.Confidence,
			PosteriorWithout:     posteriorWithout,
			MarginalContribution: fullPosterior - posteriorWithout,
		}
	}

	// Rank by absolute marginal contribution
	ranked := make([]SourceContribution, len(sources))
	copy(ranked, sources)
	for a := 0; a < len(ranked)-1; a++ {
		for b := a + 1; b < len(ranked); b++ {
			if math.Abs(ranked[b].MarginalContribution) > math.Abs(ranked[a].MarginalContribution) {
				ranked[a], ranked[b] = ranked[b], ranked[a]
			}
		}
	}
	for i := range ranked {
		ranked[i].Rank = i + 1
	}

	return &SensitivityResult{
		FullPosterior: fullPosterior,
		Prior:         prior,
		Sources:       ranked,
	}, nil
}

// AuditEntry is the full calibration report for a single composed belief.
type AuditEntry struct {
	BeliefID            string
	Content             string
	Frame               string
	DeclaredConfidence  float64
	ComputedConfidence  float64
	Discrepancy         float64
	OverconfidenceWarn  bool
	UnderconfidenceWarn bool
	Sensitivity         *SensitivityResult
}

// AuditReport collects calibration results across all composed beliefs.
type AuditReport struct {
	Entries            []*AuditEntry
	TotalBeliefs       int
	Overconfident      int
	Underconfident     int
	WellCalibrated     int
	MeanDiscrepancy    float64
	WorstBeliefID      string
	WorstDiscrepancy   float64
}

// BuildAuditReport runs sensitivity analysis over composed beliefs and assembles the full report.
func BuildAuditReport(composed []*ComposedBelief, priors map[string]float64, evidenceMap map[string][]Evidence) *AuditReport {
	report := &AuditReport{TotalBeliefs: len(composed)}
	totalDisc := 0.0

	for _, cb := range composed {
		entry := &AuditEntry{
			BeliefID:            cb.ID,
			Content:             cb.Content,
			Frame:               cb.Frame,
			DeclaredConfidence:  cb.DeclaredConfidence,
			ComputedConfidence:  cb.ComputedConfidence,
			Discrepancy:         cb.Discrepancy,
			OverconfidenceWarn:  cb.OverconfidenceWarn,
			UnderconfidenceWarn: cb.UnderconfidenceWarn,
		}

		if ev, ok := evidenceMap[cb.ID]; ok {
			if prior, ok := priors[cb.ID]; ok {
				sens, err := SensitivityAnalysis(prior, ev, cb.ComputedConfidence)
				if err == nil {
					sens.BeliefID = cb.ID
					entry.Sensitivity = sens
				}
			}
		}

		if cb.OverconfidenceWarn {
			report.Overconfident++
		} else if cb.UnderconfidenceWarn {
			report.Underconfident++
		} else {
			report.WellCalibrated++
		}

		totalDisc += cb.Discrepancy
		if cb.Discrepancy > report.WorstDiscrepancy {
			report.WorstDiscrepancy = cb.Discrepancy
			report.WorstBeliefID = cb.ID
		}

		report.Entries = append(report.Entries, entry)
	}

	if len(composed) > 0 {
		report.MeanDiscrepancy = totalDisc / float64(len(composed))
	}

	// Sort entries by discrepancy descending
	for a := 0; a < len(report.Entries)-1; a++ {
		for b := a + 1; b < len(report.Entries); b++ {
			if report.Entries[b].Discrepancy > report.Entries[a].Discrepancy {
				report.Entries[a], report.Entries[b] = report.Entries[b], report.Entries[a]
			}
		}
	}

	return report
}

// FormatAuditReport produces a human-readable audit report.
func FormatAuditReport(r *AuditReport) string {
	var sb strings.Builder
	sb.WriteString("=== Lumen Audit Report ===\n\n")
	sb.WriteString(fmt.Sprintf("Beliefs audited: %d\n", r.TotalBeliefs))
	sb.WriteString(fmt.Sprintf("Well-calibrated: %d  Overconfident: %d  Underconfident: %d\n",
		r.WellCalibrated, r.Overconfident, r.Underconfident))
	sb.WriteString(fmt.Sprintf("Mean discrepancy: %.3f  Worst: %s (%.3f)\n\n",
		r.MeanDiscrepancy, r.WorstBeliefID, r.WorstDiscrepancy))

	for _, e := range r.Entries {
		status := "✓"
		if e.OverconfidenceWarn {
			status = "⚠ OVER "
		} else if e.UnderconfidenceWarn {
			status = "⚠ UNDER"
		}
		sb.WriteString(fmt.Sprintf("%s  [%s] %s\n", status, e.Frame, e.BeliefID))
		sb.WriteString(fmt.Sprintf("     declared=%.3f  computed=%.3f  gap=%.3f\n",
			e.DeclaredConfidence, e.ComputedConfidence, e.Discrepancy))

		if e.Sensitivity != nil && len(e.Sensitivity.Sources) > 0 {
			sb.WriteString("     Sensitivity (top 3 drivers):\n")
			limit := 3
			if len(e.Sensitivity.Sources) < limit {
				limit = len(e.Sensitivity.Sources)
			}
			for _, s := range e.Sensitivity.Sources[:limit] {
				direction := "↑"
				if s.MarginalContribution < 0 {
					direction = "↓"
				}
				sb.WriteString(fmt.Sprintf("       %s %+.3f  %s (LR=%.1f conf=%.2f)\n",
					direction, s.MarginalContribution, s.SourceID, s.LikelihoodRatio, s.Confidence))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
