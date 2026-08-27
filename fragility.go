package lumen

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// FragilityEntry describes how sensitive a belief is to the loss of a single source.
type FragilityEntry struct {
	BeliefID       string
	BeliefContent  string
	CurrentConf    float64
	WeakestSource  string  // ID of the source whose loss is most damaging
	WeakestKind    string  // "record" or "belief"
	ConfWithout    float64 // estimated confidence without the weakest source
	Drop           float64 // CurrentConf - ConfWithout (positive = fragile)
	TotalSources   int
	// MinCut is the minimum number of sources that must be simultaneously removed
	// to collapse the belief confidence to zero. For N full-confidence records,
	// MinCut = N (all must be retracted). For decayed beliefs, MinCut may be 1.
	MinCut int
}

func (e FragilityEntry) String() string {
	return fmt.Sprintf("[%.0f%%→%.0f%%] %s  (weakest: %s)", e.CurrentConf*100, e.ConfWithout*100, e.BeliefID, e.WeakestSource)
}

// FragilityScan scans all active beliefs and returns them ranked by fragility:
// how much confidence would drop if their weakest source were retracted.
//
// The estimate is heuristic for beliefs without Bayesian composition metadata:
//   - 1 source:  drop to 0
//   - 2 sources: drop to half-weight of the remaining source's confidence
//   - N sources: remove the weakest and assume noisy-or on the remainder
//
// For beliefs with full Bayesian composition data the sensitivity analysis
// gives a better answer; this function uses that path when available.
type recSnap struct{ exists, retracted bool; conf float64 }

func (s *Store) FragilityScan(now time.Time) []FragilityEntry {
	s.mu.RLock()
	type snap struct {
		id          string
		content     string
		frame       Frame
		conf        float64
		state       BeliefState
		derivation  []string
		belief      *Belief  // pointer for composition metadata access
	}
	var beliefs []snap
	for id, b := range s.beliefs {
		if b.State != BeliefActive {
			continue
		}
		frame := s.frames[b.Frame]
		beliefs = append(beliefs, snap{
			id:         id,
			content:    b.Content,
			frame:      frame,
			conf:       b.CurrentConfidence(frame, now),
			state:      b.State,
			derivation: append([]string{}, b.Derivation...),
			belief:     b,
		})
	}
	// Snapshot record retraction states so fragEntryFor can run lock-free.
	// use package-level recSnap type
	records := make(map[string]recSnap, len(s.records))
	for id, r := range s.records {
		records[id] = recSnap{exists: true, retracted: r.Retracted}
	}
	// Snapshot belief confidences for source lookups.
	beliefConf := make(map[string]float64, len(s.beliefs))
	for id, b := range s.beliefs {
		f := s.frames[b.Frame]
		beliefConf[id] = b.CurrentConfidence(f, now)
	}
	s.mu.RUnlock()

	var entries []FragilityEntry
	for _, b := range beliefs {
		if len(b.derivation) == 0 {
			continue
		}
		entry := s.fragEntryFor(b.id, b.content, b.conf, b.derivation, b.frame, now, b.belief, records, beliefConf)
		if entry != nil {
			entries = append(entries, *entry)
		}
	}

	// Sort by drop descending (most fragile first), then by current confidence descending.
	sort.Slice(entries, func(i, j int) bool {
		if math.Abs(entries[i].Drop-entries[j].Drop) > 1e-6 {
			return entries[i].Drop > entries[j].Drop
		}
		return entries[i].CurrentConf > entries[j].CurrentConf
	})
	return entries
}

// sourceConf is a lightweight struct used in fragility calculations.
type sourceConf struct {
	id   string
	kind string
	conf float64 // 1.0 for non-retracted records (they don't decay); decayed for beliefs
}

// fragEntryFor computes the fragility entry for one belief.
//
// records and beliefConf are pre-snapshotted from s.records and s.beliefs;
// this function is safe to call without holding s.mu.
//
// Two paths:
//   - Composed beliefs (CompositionEvidence present): sensitivity analysis via
//     BayesianCompose, recomputing the posterior with each evidence source removed.
//     This is exact given the stored prior and evidence.
//   - Derived beliefs (no composition metadata): proportional decay approximation.
//     Models confidence as NoisyOr(sources) * decay_factor, where decay_factor =
//     current / NoisyOr(all sources). Removing source k gives:
//       estimated = NoisyOr(sources \ {k}) / NoisyOr(sources) * current
//     Valid when confidence is monotonically proportional to source noisy-or and
//     decay applies uniformly regardless of source composition (both hold in practice).
func (s *Store) fragEntryFor(beliefID, content string, currentConf float64, derivation []string, frame Frame, now time.Time, b *Belief,
	records map[string]recSnap,
	beliefConf map[string]float64,
) *FragilityEntry {
	// Path 1: Bayesian-composed belief — use sensitivity analysis (exact).
	//
	// Scale consistency: SensitivityAnalysis computes posteriors in undecayed
	// space (prior × evidence at assertion time), but currentConf is decayed.
	// We compute the undecayed full posterior, run sensitivity against it, and
	// scale results into decayed space via decayFactor = currentConf / fullPosterior.
	// This is the same proportional model norScale uses, applied consistently.
	if b != nil && len(b.CompositionEvidence) > 0 {
		fullPosterior, cerr := BayesianCompose(b.CompositionPrior, b.CompositionEvidence)
		if cerr == nil && fullPosterior > 1e-9 {
			sens, err := SensitivityAnalysis(b.CompositionPrior, b.CompositionEvidence, fullPosterior)
			if err == nil && len(sens.Sources) > 0 {
				decayFactor := currentConf / fullPosterior

				// Fragility concerns supporting evidence only: the source whose
				// removal causes the largest *drop*. Contradicting evidence (whose
				// removal raises the posterior) is not a fragility concern.
				var worst *SourceContribution
				var worstDrop float64
				for i := range sens.Sources {
					sc := &sens.Sources[i]
					d := (fullPosterior - sc.PosteriorWithout) * decayFactor
					if d > worstDrop {
						worstDrop = d
						worst = sc
					}
				}
				if worst != nil {
					// MinCut for composed beliefs: number of supporting evidence
					// blocks. Removing all of them returns the posterior to the
					// prior — with a nonzero prior the belief never collapses to
					// zero, so this counts blocks needed to lose all evidential
					// support, not blocks needed to reach zero.
					supporting := 0
					for _, ev := range b.CompositionEvidence {
						if ev.LikelihoodRatio > 1 {
							supporting++
						}
					}
					return &FragilityEntry{
						BeliefID:      beliefID,
						BeliefContent: content,
						CurrentConf:   currentConf,
						WeakestSource: worst.SourceID,
						WeakestKind:   "evidence",
						ConfWithout:   worst.PosteriorWithout * decayFactor,
						Drop:          worstDrop,
						TotalSources:  len(b.CompositionEvidence),
						MinCut:        supporting,
					}
				}
				// All evidence is contradicting — removal only raises confidence.
				// Fall through to the derivation path.
			}
		}
	}

	// Path 2: Derived belief — proportional decay approximation.
	var sources []sourceConf
	for _, srcID := range derivation {
		if rec, ok := records[srcID]; ok {
			c := 1.0
			if rec.retracted {
				c = 0
			}
			sources = append(sources, sourceConf{srcID, "record", c})
		} else if conf, ok := beliefConf[srcID]; ok {
			sources = append(sources, sourceConf{srcID, "belief", conf})
		}
	}

	if len(sources) == 0 {
		return nil
	}

	worstID   := sources[0].id
	worstKind := sources[0].kind
	worstRemConf := estimateWithout(sources, 0, currentConf)

	for i := 1; i < len(sources); i++ {
		remConf := estimateWithout(sources, i, currentConf)
		if remConf < worstRemConf {
			worstRemConf = remConf
			worstID = sources[i].id
			worstKind = sources[i].kind
		}
	}

	drop := currentConf - worstRemConf
	if drop < 0 {
		drop = 0
	}

	minCut := computeMinCut(sources, currentConf)

	return &FragilityEntry{
		BeliefID:      beliefID,
		BeliefContent: content,
		CurrentConf:   currentConf,
		WeakestSource: worstID,
		WeakestKind:   worstKind,
		ConfWithout:   worstRemConf,
		Drop:          drop,
		TotalSources:  len(sources),
		MinCut:        minCut,
	}
}

// estimateWithout estimates confidence when source at index skip is removed.
// Delegates to norScale (defined in impact.go) for the noisy-or calculation.
func estimateWithout(sources []sourceConf, skip int, currentConf float64) float64 {
	if len(sources) == 1 {
		return 0 // only source: removing it leaves zero support
	}
	without := make([]sourceConf, 0, len(sources)-1)
	for i, s := range sources {
		if i != skip { without = append(without, s) }
	}
	return norScale(without, sources, currentConf)
}

// computeMinCut finds the minimum number of sources to remove that would zero out
// confidence. It uses a greedy approach: repeatedly remove the source with the
// highest confidence (most damaging to the noisy-or) until the result is 0.
func computeMinCut(sources []sourceConf, currentConf float64) int {
	if len(sources) == 0 || currentConf == 0 {
		return 0
	}
	// Work on a copy.
	remaining := make([]sourceConf, len(sources))
	copy(remaining, sources)
	cut := 0
	for len(remaining) > 0 {
		// Remove highest-confidence source first (greedy min cut).
		best := 0
		for i := 1; i < len(remaining); i++ {
			if remaining[i].conf > remaining[best].conf {
				best = i
			}
		}
		remaining = append(remaining[:best], remaining[best+1:]...)
		cut++
		// Check noisy-or of remaining.
		p := 1.0
		for _, s := range remaining {
			p *= (1.0 - s.conf)
		}
		if 1.0-p < 1e-9 {
			return cut
		}
		if len(remaining) == 0 {
			return cut
		}
	}
	return cut
}
