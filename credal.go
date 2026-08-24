package lumen

import (
	"fmt"
	"math"
	"strings"
)

// CredalEvidence is an evidence source with interval-valued likelihood ratio.
// Instead of declaring LR=4.5 (a point), you declare LR in [lo, hi],
// representing genuine uncertainty about how diagnostic the evidence is.
//
// The credal approach: both the prior and the LR can be imprecise.
// The posterior inherits this imprecision as an interval.
type CredalEvidence struct {
	SourceID   string
	Confidence float64
	LRLo       float64 // lower bound on likelihood ratio
	LRHi       float64 // upper bound on likelihood ratio (= LRLo for point estimate)
}

// CredalPrior is an interval-valued prior: [Lo, Hi] where Lo <= Hi and both in (0,1).
// Represents genuine uncertainty about the base rate of the hypothesis.
type CredalPrior struct {
	Lo float64
	Hi float64
}

// CredalPosterior is an interval-valued posterior after Bayesian update.
// The interval [Lo, Hi] is the tight range of possible posteriors
// given the interval prior and interval likelihood ratios.
type CredalPosterior struct {
	Lo    float64
	Hi    float64
	Prior CredalPrior
}

// Width returns the imprecision of the posterior — how uncertain we are.
func (p CredalPosterior) Width() float64 {
	return p.Hi - p.Lo
}

// Midpoint returns the central estimate.
func (p CredalPosterior) Midpoint() float64 {
	return (p.Lo + p.Hi) / 2
}

// Contains checks whether a point estimate falls within the credal posterior.
func (p CredalPosterior) Contains(v float64) bool {
	return v >= p.Lo && v <= p.Hi
}

// bayesUpdatePoint is the standard single-step Bayesian update via log-odds.
// Returns the posterior for a given prior and likelihood ratio.
// Both prior and lr must be positive; prior must be in (0,1).
func bayesUpdatePoint(prior, lr float64) float64 {
	logOdds := math.Log(prior/(1-prior)) + math.Log(lr)
	return 1 / (1 + math.Exp(-logOdds))
}

// CredalBayesUpdate computes the posterior credal set given an interval prior
// and a set of evidence with interval-valued likelihood ratios.
//
// Theoretical basis:
// The Bayesian update function f(p, LR) = p*LR / (p*LR + (1-p)) is
// monotonically increasing in both p (for LR > 0) and LR (for p in (0,1)).
// Therefore the posterior credal set's tight interval is achieved at the
// corners of the (prior, LR) rectangle — no interior extrema exist.
//
// For multiple evidence sources with interval LRs [lo_i, hi_i]:
// The combined LR is the product of individual LRs (assuming independence).
// The posterior interval is computed by updating the prior interval endpoints
// with the combined LR interval endpoints.
//
// Confidence weighting is applied to each LR:
//   effective_LR = 1 + confidence * (LR - 1)
// For interval LRs, this is applied to both endpoints.
func CredalBayesUpdate(prior CredalPrior, evidence []CredalEvidence) (CredalPosterior, error) {
	if prior.Lo <= 0 || prior.Hi >= 1 || prior.Lo > prior.Hi {
		return CredalPosterior{}, fmt.Errorf("invalid credal prior [%.4f, %.4f]: must have 0 < lo <= hi < 1",
			prior.Lo, prior.Hi)
	}
	for _, e := range evidence {
		if e.LRLo <= 0 || e.LRHi < e.LRLo {
			return CredalPosterior{}, fmt.Errorf("invalid LR interval [%.4f, %.4f] for %s",
				e.LRLo, e.LRHi, e.SourceID)
		}
		if e.Confidence <= 0 || e.Confidence > 1 {
			return CredalPosterior{}, fmt.Errorf("invalid confidence %.4f for %s",
				e.Confidence, e.SourceID)
		}
	}

	// Compute combined log-odds contribution from all evidence.
	// Combined effective LR interval [effLo, effHi] accumulated in log-odds space.
	combinedLogOddsLo := 0.0
	combinedLogOddsHi := 0.0
	for _, e := range evidence {
		effLRLo := 1 + e.Confidence*(e.LRLo-1)
		effLRHi := 1 + e.Confidence*(e.LRHi-1)
		combinedLogOddsLo += math.Log(effLRLo)
		combinedLogOddsHi += math.Log(effLRHi)
	}

	// Apply to prior endpoints.
	// Monotonicity: posterior is increasing in both prior p and in combined LR.
	// Min posterior: smallest prior, smallest combined LR.
	// Max posterior: largest prior, largest combined LR.
	priorLogOddsLo := math.Log(prior.Lo / (1 - prior.Lo))
	priorLogOddsHi := math.Log(prior.Hi / (1 - prior.Hi))

	posteriorLo := 1 / (1 + math.Exp(-(priorLogOddsLo + combinedLogOddsLo)))
	posteriorHi := 1 / (1 + math.Exp(-(priorLogOddsHi + combinedLogOddsHi)))

	// Ensure ordering (should always hold by monotonicity, but guard for float precision)
	if posteriorLo > posteriorHi {
		posteriorLo, posteriorHi = posteriorHi, posteriorLo
	}

	return CredalPosterior{
		Lo:    posteriorLo,
		Hi:    posteriorHi,
		Prior: prior,
	}, nil
}

// CredalImprecisionReport describes how imprecision evolved through the update.
type CredalImprecisionReport struct {
	PriorWidth     float64
	PosteriorWidth float64
	Reduction      float64 // PriorWidth - PosteriorWidth (positive = imprecision reduced)
	PriorSources   []string // which sources contributed imprecision via interval LRs
	Posterior      CredalPosterior
	PointComparison float64 // result of standard BayesianCompose with midpoint prior/LRs
}

// CredalAudit runs the full credal update and compares with the point-prior result.
// It shows both what the posterior interval is and how the imprecision evolved.
func CredalAudit(prior CredalPrior, evidence []CredalEvidence) (*CredalImprecisionReport, error) {
	posterior, err := CredalBayesUpdate(prior, evidence)
	if err != nil {
		return nil, err
	}

	// Point comparison: use midpoint prior and midpoint LRs
	midPrior := prior.Midpoint()
	pointEvidence := make([]Evidence, len(evidence))
	for i, e := range evidence {
		pointEvidence[i] = Evidence{
			SourceID:        e.SourceID,
			Confidence:      e.Confidence,
			LikelihoodRatio: (e.LRLo + e.LRHi) / 2,
		}
	}
	pointResult, _ := BayesianCompose(midPrior, pointEvidence)

	var intervalSources []string
	for _, e := range evidence {
		if e.LRHi > e.LRLo {
			intervalSources = append(intervalSources, fmt.Sprintf("%s [%.1f, %.1f]",
				e.SourceID, e.LRLo, e.LRHi))
		}
	}

	return &CredalImprecisionReport{
		PriorWidth:      prior.Hi - prior.Lo,
		PosteriorWidth:  posterior.Width(),
		Reduction:       (prior.Hi - prior.Lo) - posterior.Width(),
		PriorSources:    intervalSources,
		Posterior:       posterior,
		PointComparison: pointResult,
	}, nil
}

// FormatCredalReport returns a human-readable credal audit.
func FormatCredalReport(r *CredalImprecisionReport) string {
	var sb strings.Builder
	sb.WriteString("Credal (imprecise probability) analysis:\n")
	sb.WriteString(fmt.Sprintf("  Prior interval: [%.3f, %.3f]  (width=%.3f)\n",
		r.Posterior.Prior.Lo, r.Posterior.Prior.Hi, r.PriorWidth))

	if len(r.PriorSources) > 0 {
		sb.WriteString("  Interval LR sources (contribute to posterior width):\n")
		for _, s := range r.PriorSources {
			sb.WriteString("    " + s + "\n")
		}
	}

	sb.WriteString(fmt.Sprintf("  Posterior interval: [%.3f, %.3f]  (width=%.3f)\n",
		r.Posterior.Lo, r.Posterior.Hi, r.PosteriorWidth))
	sb.WriteString(fmt.Sprintf("  Imprecision %s by %.3f  (evidence %s uncertainty)\n",
		func() string {
			if r.Reduction >= 0 {
				return "reduced"
			}
			return "increased"
		}(),
		math.Abs(r.Reduction),
		func() string {
			if r.Reduction >= 0 {
				return "resolved"
			}
			return "introduced"
		}()))
	sb.WriteString(fmt.Sprintf("  Point estimate (midpoint prior, midpoint LR): %.3f\n", r.PointComparison))
	sb.WriteString(fmt.Sprintf("  Point inside credal interval: %v\n",
		r.Posterior.Contains(r.PointComparison)))
	sb.WriteString(fmt.Sprintf("  Midpoint of credal interval: %.3f\n", r.Posterior.Midpoint()))
	return sb.String()
}

// CredalDecayFactor computes a decay factor as a credal interval rather than a point.
// When the belief's age is uncertain (because assertion time is approximate) or
// when the halflife itself is uncertain, the decay factor becomes an interval.
//
// This models: "I know this was asserted sometime in the last hour, and I believe
// the halflife is between 20 and 40 days."
type CredalDecay struct {
	AgeMin    float64 // minimum elapsed time in hours
	AgeMax    float64 // maximum elapsed time in hours
	HalflifeMin float64 // minimum halflife in hours
	HalflifeMax float64 // maximum halflife in hours
}

// DecayInterval returns [min_factor, max_factor] for the credal decay.
// Factor = 0.5^(age/halflife). This is decreasing in age and increasing in halflife.
// So: min factor at (max age, min halflife), max factor at (min age, max halflife).
func (d CredalDecay) DecayInterval() (float64, float64) {
	minFactor := math.Pow(0.5, d.AgeMax/d.HalflifeMin)
	maxFactor := math.Pow(0.5, d.AgeMin/d.HalflifeMax)
	return minFactor, maxFactor
}

// Midpoint returns the center of the credal prior interval.
func (p CredalPrior) Midpoint() float64 {
	return (p.Lo + p.Hi) / 2
}
