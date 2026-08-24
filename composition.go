package lumen

import (
	"fmt"
	"math"
	"strings"
)

// Evidence represents a piece of evidence and how much it shifts belief.
// LikelihoodRatio is P(evidence | hypothesis true) / P(evidence | hypothesis false).
// A ratio > 1 supports the hypothesis; < 1 undermines it; = 1 is neutral.
type Evidence struct {
	SourceID       string
	Confidence     float64 // confidence in the source belief/record
	LikelihoodRatio float64 // how much this evidence shifts the posterior
}

// BayesianCompose computes the posterior probability of a hypothesis
// given a prior and a set of evidence items with likelihood ratios.
//
// Uses the log-odds form for numerical stability:
//   log_odds(posterior) = log_odds(prior) + sum(log(LR_i * conf_i + (1 - conf_i)))
//
// The confidence weighting on each LR reflects that uncertain sources
// contribute less diagnostic power: a 50% confident source with LR=10
// contributes less than a 95% confident source with LR=10.
func BayesianCompose(prior float64, evidence []Evidence) (float64, error) {
	if prior <= 0 || prior >= 1 {
		return 0, fmt.Errorf("prior must be in (0, 1), got %.4f", prior)
	}
	for _, e := range evidence {
		if e.LikelihoodRatio <= 0 {
			return 0, fmt.Errorf("likelihood ratio must be positive, got %.4f for %s", e.LikelihoodRatio, e.SourceID)
		}
		if e.Confidence <= 0 || e.Confidence > 1 {
			return 0, fmt.Errorf("confidence must be in (0, 1], got %.4f for %s", e.Confidence, e.SourceID)
		}
	}

	logOdds := math.Log(prior / (1 - prior))
	for _, e := range evidence {
		// Weighted LR: if confidence is c, the effective LR is
		// c * LR + (1 - c) * 1 = 1 + c*(LR - 1)
		// This interpolates between LR=1 (no information) at conf=0
		// and LR at conf=1.
		effectiveLR := 1 + e.Confidence*(e.LikelihoodRatio-1)
		logOdds += math.Log(effectiveLR)
	}

	// Convert back from log-odds
	posterior := 1 / (1 + math.Exp(-logOdds))
	return posterior, nil
}

// DempsterShaferMass represents a basic probability assignment in DS theory.
// It assigns probability mass to subsets of {true, false, unknown}.
type DempsterShaferMass struct {
	SourceID    string
	MassTrue    float64 // mass assigned to {hypothesis is true}
	MassFalse   float64 // mass assigned to {hypothesis is false}
	MassUnknown float64 // mass assigned to {true, false} — total ignorance
}

// Normalize ensures masses sum to 1.
func (m *DempsterShaferMass) Normalize() error {
	total := m.MassTrue + m.MassFalse + m.MassUnknown
	if math.Abs(total-1.0) > 0.001 {
		return fmt.Errorf("DS masses for %s sum to %.4f, must sum to 1.0", m.SourceID, total)
	}
	m.MassTrue /= total
	m.MassFalse /= total
	m.MassUnknown /= total
	return nil
}

// DempsterShaferCompose combines two mass functions using Dempster's rule.
// Returns (belief in true, plausibility of true, conflict K).
// High K indicates the evidence sources are contradicting each other.
func DempsterShaferCompose(m1, m2 DempsterShaferMass) (belief, plausibility, conflict float64, err error) {
	// Dempster's rule for two sources over {T, F, Θ} where Θ = {T,F}:
	// Focal elements after combination (before normalization by 1-K):
	//   {T}: m1(T)*m2(T) + m1(T)*m2(Θ) + m1(Θ)*m2(T)
	//   {F}: m1(F)*m2(F) + m1(F)*m2(Θ) + m1(Θ)*m2(F)
	//   {Θ}: m1(Θ)*m2(Θ)
	// Conflict K: m1(T)*m2(F) + m1(F)*m2(T)

	K := m1.MassTrue*m2.MassFalse + m1.MassFalse*m2.MassTrue
	if K >= 1.0 {
		return 0, 0, K, fmt.Errorf("total conflict (K=%.4f): sources %q and %q are completely contradictory", K, m1.SourceID, m2.SourceID)
	}

	norm := 1 - K
	massT := (m1.MassTrue*m2.MassTrue + m1.MassTrue*m2.MassUnknown + m1.MassUnknown*m2.MassTrue) / norm
	_ = (m1.MassFalse*m2.MassFalse + m1.MassFalse*m2.MassUnknown + m1.MassUnknown*m2.MassFalse) / norm
	_ = (m1.MassUnknown * m2.MassUnknown) / norm // massTheta

	// Belief in T: sum of all mass subsets that are subsets of {T}
	// In our 3-element model: just massT
	belief = massT
	// Plausibility of T: sum of all mass subsets that intersect {T}
	// = massT + massTheta (since {T,F} intersects {T})
	plausibility = massT + (m1.MassUnknown*m2.MassUnknown)/norm
	conflict = K

	return belief, plausibility, conflict, nil
}

// ComposedBelief wraps a belief with explicit composition metadata.
type ComposedBelief struct {
	ID                 string
	Content            string
	Frame              string
	Prior              float64
	Evidence           []Evidence
	ComputedConfidence float64
	DeclaredConfidence float64
	Discrepancy        float64 // |computed - declared|; 0 means asserter is well-calibrated
	OverconfidenceWarn bool    // true if declared > computed by threshold
	UnderconfidenceWarn bool   // true if declared < computed by threshold
	Method             string  // "bayesian" or "dempster-shafer"
}

// ValidateConfidence checks whether the asserter's declared confidence
// is consistent with what the Bayesian computation would produce.
func ValidateConfidence(declared float64, prior float64, evidence []Evidence) (*ComposedBelief, error) {
	computed, err := BayesianCompose(prior, evidence)
	if err != nil {
		return nil, err
	}

	cb := &ComposedBelief{
		Prior:              prior,
		Evidence:           evidence,
		ComputedConfidence: computed,
		DeclaredConfidence: declared,
		Discrepancy:        math.Abs(computed - declared),
		Method:             "bayesian",
	}

	const threshold = 0.15
	if declared > computed+threshold {
		cb.OverconfidenceWarn = true
	}
	if declared < computed-threshold {
		cb.UnderconfidenceWarn = true
	}

	return cb, nil
}

// FormatComposed returns a human-readable summary of a composed belief.
func FormatComposed(cb *ComposedBelief) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Composition analysis (%s):\n", cb.Method))
	sb.WriteString(fmt.Sprintf("  Prior: %.3f\n", cb.Prior))
	sb.WriteString("  Evidence:\n")
	for _, e := range cb.Evidence {
		direction := "supports"
		if e.LikelihoodRatio < 1 {
			direction = "undermines"
		}
		sb.WriteString(fmt.Sprintf("    [%s] LR=%.2f (conf=%.2f) → %s hypothesis\n",
			e.SourceID, e.LikelihoodRatio, e.Confidence, direction))
	}
	sb.WriteString(fmt.Sprintf("  Computed posterior: %.3f\n", cb.ComputedConfidence))
	sb.WriteString(fmt.Sprintf("  Declared confidence: %.3f\n", cb.DeclaredConfidence))
	sb.WriteString(fmt.Sprintf("  Discrepancy: %.3f", cb.Discrepancy))
	if cb.OverconfidenceWarn {
		sb.WriteString(" ⚠ OVERCONFIDENT (declared significantly exceeds computed)")
	} else if cb.UnderconfidenceWarn {
		sb.WriteString(" ⚠ UNDERCONFIDENT (declared significantly below computed)")
	} else {
		sb.WriteString(" ✓ well-calibrated")
	}
	sb.WriteString("\n")
	return sb.String()
}

// CorrelatedEvidence extends Evidence with an explicit correlation coefficient
// to other evidence sources. When two pieces of evidence share the same
// underlying source, combining them as independent overstates the update.
type CorrelatedEvidence struct {
	Evidence
	// CorrelationWith maps source IDs to Pearson correlation coefficients [0,1].
	// 0 = independent, 1 = identical (would double-count if treated independently).
	CorrelationWith map[string]float64
}

// BayesianComposeCorrelated adjusts for pairwise correlations between evidence sources.
// For each pair (i, j) with correlation r, we reduce the joint log-odds contribution
// by a factor derived from their overlap: effectively treating the pair as contributing
// (2 - r) independent updates rather than 2.
//
// This is a heuristic adjustment, not a full multivariate model — it prevents the
// most egregious double-counting without requiring a complete joint distribution.
func BayesianComposeCorrelated(prior float64, evidence []CorrelatedEvidence) (float64, error) {
	if prior <= 0 || prior >= 1 {
		return 0, fmt.Errorf("prior must be in (0, 1), got %.4f", prior)
	}
	for _, e := range evidence {
		if e.LikelihoodRatio <= 0 {
			return 0, fmt.Errorf("likelihood ratio must be positive for %s", e.SourceID)
		}
	}

	// Compute per-source effective weight after deducting shared weight with correlated sources.
	// For source i: effective_weight_i = 1 - (sum of r_ij for all j != i) / 2
	// This ensures correlated pairs together contribute < 2 independent updates.
	weights := make([]float64, len(evidence))
	for i, ei := range evidence {
		totalCorr := 0.0
		for j, ej := range evidence {
			if i == j {
				continue
			}
			if r, ok := ei.CorrelationWith[ej.SourceID]; ok {
				totalCorr += r
			}
		}
		// Discount: correlated sources share evidential weight
		weights[i] = 1.0 - (totalCorr / 2.0)
		if weights[i] < 0.1 {
			weights[i] = 0.1 // floor: even fully correlated sources contribute something
		}
	}

	logOdds := math.Log(prior / (1 - prior))
	for i, e := range evidence {
		effectiveLR := 1 + e.Confidence*(e.LikelihoodRatio-1)
		// Scale log contribution by the source's effective independent weight
		logOdds += weights[i] * math.Log(effectiveLR)
	}

	posterior := 1 / (1 + math.Exp(-logOdds))
	return posterior, nil
}

// EvidenceCorrelationReport describes how correlation adjustments affected a computation.
type EvidenceCorrelationReport struct {
	NaivePosterior      float64
	AdjustedPosterior   float64
	OvercountingReduced float64 // naive - adjusted; positive means we were double-counting
	SourceWeights       map[string]float64
}

// CompareNaiveVsCorrelated runs both methods and reports the difference.
func CompareNaiveVsCorrelated(prior float64, evidence []CorrelatedEvidence) (*EvidenceCorrelationReport, error) {
	// Naive: ignore correlations
	naiveEvidence := make([]Evidence, len(evidence))
	for i, e := range evidence {
		naiveEvidence[i] = e.Evidence
	}
	naive, err := BayesianCompose(prior, naiveEvidence)
	if err != nil {
		return nil, err
	}

	adjusted, err := BayesianComposeCorrelated(prior, evidence)
	if err != nil {
		return nil, err
	}

	weights := make(map[string]float64)
	for i, ei := range evidence {
		totalCorr := 0.0
		for j, ej := range evidence {
			if i == j { continue }
			if r, ok := ei.CorrelationWith[ej.SourceID]; ok {
				totalCorr += r
			}
		}
		w := 1.0 - (totalCorr / 2.0)
		if w < 0.1 { w = 0.1 }
		weights[ei.SourceID] = w
	}

	return &EvidenceCorrelationReport{
		NaivePosterior:    naive,
		AdjustedPosterior: adjusted,
		OvercountingReduced: naive - adjusted,
		SourceWeights:     weights,
	}, nil
}
