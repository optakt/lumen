package lumen

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ConfidenceInterval represents an epistemic uncertainty range [Lo, Hi].
// A point estimate p is represented as [p, p].
// A fully uncertain belief is [0, 1].
type ConfidenceInterval struct {
	Lo float64
	Hi float64
}

func (ci ConfidenceInterval) String() string {
	if math.Abs(ci.Hi-ci.Lo) < 0.001 {
		return fmt.Sprintf("%.2f", ci.Lo)
	}
	return fmt.Sprintf("[%.2f, %.2f]", ci.Lo, ci.Hi)
}

// Midpoint returns the midpoint of the interval.
func (ci ConfidenceInterval) Midpoint() float64 {
	return (ci.Lo + ci.Hi) / 2
}

// Width returns the width of the interval (epistemic uncertainty).
func (ci ConfidenceInterval) Width() float64 {
	return ci.Hi - ci.Lo
}

// IntervalFromPoint wraps a point estimate as a degenerate interval.
func IntervalFromPoint(p float64) ConfidenceInterval {
	return ConfidenceInterval{Lo: p, Hi: p}
}

// IntervalNoisyOr combines two intervals using the noisy-or model:
// P(A∨B) = 1 - (1-pA)(1-pB), applied pointwise to corners.
// The result is the smallest interval containing all possible combinations.
func IntervalNoisyOr(a, b ConfidenceInterval) ConfidenceInterval {
	// Monotone in both arguments, so corners suffice
	lo := 1 - (1-a.Lo)*(1-b.Lo)
	hi := 1 - (1-a.Hi)*(1-b.Hi)
	return ConfidenceInterval{Lo: lo, Hi: hi}
}

// IntervalMin returns the conservative combination: [min(aLo,bLo), min(aHi,bHi)].
func IntervalMin(a, b ConfidenceInterval) ConfidenceInterval {
	return ConfidenceInterval{
		Lo: math.Min(a.Lo, b.Lo),
		Hi: math.Min(a.Hi, b.Hi),
	}
}

// IntervalDecay applies exponential decay to an interval.
// Both endpoints decay at the same rate.
func IntervalDecay(ci ConfidenceInterval, elapsed, halflife float64) ConfidenceInterval {
	if halflife <= 0 { return ci }
	factor := math.Exp(-elapsed / halflife * math.Ln2)
	return ConfidenceInterval{
		Lo: ci.Lo * factor,
		Hi: ci.Hi * factor,
	}
}

// ChainInterval holds the interval-valued confidence for a node in a provenance chain.
type ChainInterval struct {
	NodeID   string
	Kind     string
	Interval ConfidenceInterval
	Depth    int
}

// IntervalChain computes interval-valued confidence for a belief and all its ancestors.
//
// Uses credal Bayesian updating (CredalBayesUpdate) to propagate uncertainty
// through derivation chains — the same formal machinery as credal.go.
// This replaces the prior informal Lo×Lo / Hi×Hi multiplication, which had no
// correct derivation and gave wrong results for chains with multiple parents.
//
// Model:
//   - Records are point intervals: [1,1] (valid) or [0,0] (retracted).
//   - Each belief node takes its declared confidence as a prior interval [p,p]
//     and applies each parent's interval as a credal likelihood update.
//   - When a belief has multiple parents, their intervals are combined via
//     IntervalNoisyOr before being passed to CredalBayesUpdate.
//   - The result is the smallest interval consistent with all provenance uncertainty.
//
// Formal guarantee: at correlation 0 (independent sources), the update is
// standard Bayesian; at full correlation (identical sources), it degrades
// gracefully. Sequential update equals batch update (log-odds product).
func (s *Store) IntervalChain(beliefID string, now time.Time) (map[string]ChainInterval, error) {
	chain, err := s.ProvenanceChain(beliefID, now)
	if err != nil {
		return nil, err
	}

	intervals := make(map[string]ChainInterval, len(chain.Nodes))

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Process leaves-first (deepest depth first).
	order := make([]string, len(chain.DepthOrder))
	copy(order, chain.DepthOrder)
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	for _, nodeID := range order {
		node := chain.Nodes[nodeID]

		if node.Kind == "record" {
			if node.Retracted {
				intervals[nodeID] = ChainInterval{nodeID, "record", ConfidenceInterval{0, 0}, node.Depth}
			} else {
				intervals[nodeID] = ChainInterval{nodeID, "record", ConfidenceInterval{1, 1}, node.Depth}
			}
			continue
		}

		// Opaque frames: treat the declared confidence as the authoritative interval.
		// The internal composition of an opaque frame is not accessible for chain propagation.
		if blObj, blOk := s.beliefs[nodeID]; blOk {
			if frameObj, fOk := s.frames[blObj.Frame]; fOk && frameObj.IsOpaque() {
				declared := blObj.CurrentConfidence(frameObj, now)
				intervals[nodeID] = ChainInterval{nodeID, "belief", ConfidenceInterval{Lo: declared, Hi: declared}, node.Depth}
				continue
			}
		}

				// Belief: start with declared confidence as a degenerate prior interval.
		var declaredConf float64
		if bl, ok := s.beliefs[nodeID]; ok {
			frame := s.frames[bl.Frame]
			declaredConf = bl.CurrentConfidence(frame, now)
		} else {
			declaredConf = node.Confidence
		}

		priorInterval := CredalPrior{Lo: declaredConf, Hi: declaredConf}

		if len(node.Parents) > 0 {
			// Combine parent intervals with IntervalNoisyOr, then use the
			// combined interval as the evidence likelihood ratio bounds.
			var parentInterval ConfidenceInterval
			first := true
			for _, parentID := range node.Parents {
				if ci, ok := intervals[parentID]; ok {
					if first {
						parentInterval = ci.Interval
						first = false
					} else {
						parentInterval = IntervalNoisyOr(parentInterval, ci.Interval)
					}
				}
			}

			if !first {
				if parentInterval.Hi <= 0.0 {
					// All parent sources are retracted — this belief has no support.
					priorInterval = CredalPrior{Lo: 0, Hi: 0}
				} else if parentInterval.Lo < 0.999 || parentInterval.Hi < 0.999 {
					// Treat parent support as credal evidence via CredalBayesUpdate.
					// Convert parent interval [lo, hi] to likelihood ratio bounds:
					// LR = p(evidence|H) / p(evidence|¬H) ≈ support / (1 - support)
					// LRLo is clamped to eps to avoid CredalBayesUpdate rejecting LR=0.
					// We handle the LR=0 (full retraction) case above.
					eps := 1e-6
					lrLo := math.Max(eps, math.Min(100, parentInterval.Lo/(1-parentInterval.Lo+eps)))
					lrHi := math.Min(100, parentInterval.Hi/(1-parentInterval.Hi+eps))
					if lrHi < lrLo { lrHi = lrLo }
					// CredalBayesUpdate also requires Confidence > 0; we pass 1.0 (fully observed).
					evidence := []CredalEvidence{{
						LRLo:       lrLo,
						LRHi:       lrHi,
						Confidence: 1.0,
					}}
					// Guard prior endpoints away from 0 and 1 (CredalBayesUpdate constraint).
					guardedPrior := CredalPrior{
						Lo: math.Max(eps, math.Min(1-eps, priorInterval.Lo)),
						Hi: math.Max(eps, math.Min(1-eps, priorInterval.Hi)),
					}
					posterior, err := CredalBayesUpdate(guardedPrior, evidence)
					if err == nil {
						priorInterval = CredalPrior{Lo: posterior.Lo, Hi: posterior.Hi}
					}
				}
			}
		}

		intervals[nodeID] = ChainInterval{
			NodeID:   nodeID,
			Kind:     "belief",
			Interval: ConfidenceInterval{Lo: priorInterval.Lo, Hi: priorInterval.Hi},
			Depth:    node.Depth,
		}
	}

	return intervals, nil
}


// IntervalSummary renders the interval chain as a readable report.
func IntervalSummary(belief string, intervals map[string]ChainInterval, chain *ProvenanceChain) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Interval analysis for: %s\n\n", belief)

	// Root first
	if ci, ok := intervals[belief]; ok {
		fmt.Fprintf(&b, "Declared confidence: %s\n", ci.Interval)
		fmt.Fprintf(&b, "Epistemic uncertainty width: %.1f%%\n\n", ci.Interval.Width()*100)
	}

	// Show paths
	paths := chain.ConfidencePaths()
	for i, p := range paths {
		fmt.Fprintf(&b, "Path %d:\n", i+1)
		for _, step := range p.Steps {
			ci := intervals[step.ID]
			node := chain.Nodes[step.ID]
			indent := strings.Repeat("  ", node.Depth)
			fmt.Fprintf(&b, "%s%s: %s\n", indent, step.ID, ci.Interval)
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

// intervalChainSkip runs the same bottom-up propagation as IntervalChain but
// treats skipID as absent (confidence [0,0]).  skipID="" runs normally.
// The caller holds no lock; this function takes its own RLock.
func (s *Store) intervalChainSkip(beliefID, skipID string, chain *ProvenanceChain, now time.Time) (ConfidenceInterval, error) {
	intervals := make(map[string]ChainInterval, len(chain.Nodes))

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Process leaves-first (deepest first).
	order := make([]string, len(chain.DepthOrder))
	copy(order, chain.DepthOrder)
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	eps := 1e-6

	for _, nodeID := range order {
		node := chain.Nodes[nodeID]

		// Excluded source: treat as absent.
		if nodeID == skipID {
			intervals[nodeID] = ChainInterval{nodeID, "excluded", ConfidenceInterval{0, 0}, node.Depth}
			continue
		}

		if node.Kind == "record" {
			if node.Retracted {
				intervals[nodeID] = ChainInterval{nodeID, "record", ConfidenceInterval{0, 0}, node.Depth}
			} else {
				intervals[nodeID] = ChainInterval{nodeID, "record", ConfidenceInterval{1, 1}, node.Depth}
			}
			continue
		}

		// Opaque frames: use declared confidence as authoritative; do not propagate parents.
		if blObj, blOk := s.beliefs[nodeID]; blOk {
			if frameObj, fOk := s.frames[blObj.Frame]; fOk && frameObj.IsOpaque() {
				declared := blObj.CurrentConfidence(frameObj, now)
				intervals[nodeID] = ChainInterval{nodeID, "belief", ConfidenceInterval{Lo: declared, Hi: declared}, node.Depth}
				continue
			}
		}

		// Belief node — same logic as IntervalChain.
		var declaredConf float64
		if bl, ok := s.beliefs[nodeID]; ok {
			frame := s.frames[bl.Frame]
			declaredConf = bl.CurrentConfidence(frame, now)
		} else {
			declaredConf = node.Confidence
		}
		priorInterval := CredalPrior{Lo: declaredConf, Hi: declaredConf}

		if len(node.Parents) > 0 {
			var parentInterval ConfidenceInterval
			first := true
			for _, parentID := range node.Parents {
				if ci, ok := intervals[parentID]; ok {
					if first {
						parentInterval = ci.Interval
						first = false
					} else {
						parentInterval = IntervalNoisyOr(parentInterval, ci.Interval)
					}
				}
			}

			if !first {
				if parentInterval.Hi <= 0.0 {
					priorInterval = CredalPrior{Lo: 0, Hi: 0}
				} else if parentInterval.Lo < 0.999 || parentInterval.Hi < 0.999 {
					lrLo := math.Max(eps, math.Min(100, parentInterval.Lo/(1-parentInterval.Lo+eps)))
					lrHi := math.Min(100, parentInterval.Hi/(1-parentInterval.Hi+eps))
					if lrHi < lrLo {
						lrHi = lrLo
					}
					evidence := []CredalEvidence{{
						LRLo:       lrLo,
						LRHi:       lrHi,
						Confidence: 1.0,
					}}
					guardedPrior := CredalPrior{
						Lo: math.Max(eps, math.Min(1-eps, priorInterval.Lo)),
						Hi: math.Max(eps, math.Min(1-eps, priorInterval.Hi)),
					}
					posterior, pErr := CredalBayesUpdate(guardedPrior, evidence)
					if pErr == nil {
						priorInterval = CredalPrior{Lo: posterior.Lo, Hi: posterior.Hi}
					}
				}
			}
		}
		intervals[nodeID] = ChainInterval{nodeID, "belief", ConfidenceInterval{Lo: priorInterval.Lo, Hi: priorInterval.Hi}, node.Depth}
	}

	ci, ok := intervals[beliefID]
	if !ok {
		return ConfidenceInterval{}, fmt.Errorf("belief %q not found in interval computation", beliefID)
	}
	return ci.Interval, nil
}

// CounterfactualConfidence computes what a belief's confidence interval would be
// if a specific source (record or belief ID) were excluded from the derivation chain.
//
// Returns: (full interval, counterfactual interval, midpoint delta, error).
// delta = counterfactual_midpoint - full_midpoint.
// A large negative delta means the excluded source is load-bearing: without it,
// confidence drops significantly.
func (s *Store) CounterfactualConfidence(beliefID, excludedID string, now time.Time) (
	full, counterfactual ConfidenceInterval, delta float64, err error,
) {
	chain, cErr := s.ProvenanceChain(beliefID, now)
	if cErr != nil {
		return full, counterfactual, 0, cErr
	}
	if _, exists := chain.Nodes[excludedID]; !exists {
		return full, counterfactual, 0, fmt.Errorf("source %q not in provenance chain for %q", excludedID, beliefID)
	}

	full, err = s.intervalChainSkip(beliefID, "", chain, now)
	if err != nil {
		return
	}
	counterfactual, err = s.intervalChainSkip(beliefID, excludedID, chain, now)
	if err != nil {
		return
	}

	fullMid := (full.Lo + full.Hi) / 2
	cfMid := (counterfactual.Lo + counterfactual.Hi) / 2
	delta = cfMid - fullMid
	return
}
