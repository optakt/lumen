package lumen

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// CorrelationAwareMerge merges two beliefs using correlation-adjusted confidence.
//
// Standard noisy-or assumes the two beliefs are independent — their combined
// confidence is 1-(1-pA)(1-pB). But if bA and bB share sources, this
// overcounts the evidence. The overlap in their derivation chains creates
// correlation that should reduce the combined confidence.
//
// This function computes the overlap coefficient (|A∩B| / |A∪B|) of the
// derivation chains and applies a correlation correction:
//
//   r ≈ overlap_coefficient * max_correlation (default max_correlation = 0.8)
//   adjusted_confidence = noisy_or(pA, pB) * (1 - r)^0.5
//
// If the derivation chains are completely disjoint (no shared sources),
// the result is equivalent to standard noisy-or.
// If the chains are identical (complete overlap), confidence collapses to
// a weighted average, not compounded.
func (s *Store) CorrelationAwareMerge(
	beliefAID, beliefBID, mergedID, content, frame string,
	retire bool, now time.Time,
) (*MergeResult, error) {
	s.mu.RLock()
	bA, okA := s.beliefs[beliefAID]
	bB, okB := s.beliefs[beliefBID]
	if !okA { s.mu.RUnlock(); return nil, fmt.Errorf("belief %s not found", beliefAID) }
	if !okB { s.mu.RUnlock(); return nil, fmt.Errorf("belief %s not found", beliefBID) }

	frameA := s.frames[bA.Frame]
	frameB := s.frames[bB.Frame]
	confA := bA.CurrentConfidence(frameA, now)
	confB := bB.CurrentConfidence(frameB, now)
	derivA := append([]string{}, bA.Derivation...)
	derivB := append([]string{}, bB.Derivation...)
	s.mu.RUnlock()

	// Compute overlap in derivation chains (transitively — full ancestry)
	chainA, err := s.ProvenanceChain(beliefAID, now)
	if err != nil { return nil, err }
	chainB, err := s.ProvenanceChain(beliefBID, now)
	if err != nil { return nil, err }

	// Get leaf records for each chain
	recordsA := chainLeafRecords(chainA)
	recordsB := chainLeafRecords(chainB)

	// Jaccard overlap of record sets
	intersect := 0
	for r := range recordsA {
		if recordsB[r] { intersect++ }
	}
	union := len(recordsA) + len(recordsB) - intersect
	overlap := 0.0
	if union > 0 {
		overlap = float64(intersect) / float64(union)
	}

	// Correlation estimate from overlap
	const maxCorrelation = 0.8
	r := overlap * maxCorrelation

	// Noisy-or then correlation adjustment
	noisyOr := 1 - (1-confA)*(1-confB)
	// Adjust: high correlation reduces the compounding
	// At r=0: no adjustment. At r=1: confidence = average (no compounding benefit)
	avgConf := (confA + confB) / 2
	adjustedConf := noisyOr*(1-r) + avgConf*r

	methodDesc := fmt.Sprintf("correlation-aware merge: noisy-or=%.2f, r=%.2f (overlap=%.0f%%), adjusted=%.2f",
		noisyOr, r, overlap*100, adjustedConf)

	// Build union derivation
	derivSeen := make(map[string]bool)
	var derivUnion []string
	for _, id := range append(derivA, derivB...) {
		if !derivSeen[id] { derivSeen[id] = true; derivUnion = append(derivUnion, id) }
	}
	derivUnion = append(derivUnion, beliefAID, beliefBID)

	targetFrame := frame
	if targetFrame == "" { targetFrame = bA.Frame }

	merged := &Belief{
		ID:         mergedID,
		Frame:      targetFrame,
		Content:    content,
		Confidence: adjustedConf,
		AssertedAt: now,
		Derivation: derivUnion,
		State:      BeliefActive,
	}
	if err := s.Believe(merged); err != nil {
		return nil, fmt.Errorf("create merged belief: %w", err)
	}

	var retired []string
	if retire {
		s.mu.Lock()
		for _, id := range []string{beliefAID, beliefBID} {
			if b, ok := s.beliefs[id]; ok {
				b.State = BeliefSuspect
				b.Content = "[SUPERSEDED] " + b.Content
			}
		}
		s.mu.Unlock()
		retired = []string{beliefAID, beliefBID}
	}

	return &MergeResult{
		MergedID:           mergedID,
		RetiredIDs:         retired,
		CombinedConfidence: adjustedConf,
		CombinationMethod:  methodDesc,
	}, nil
}

// chainLeafRecords returns the set of record IDs at the leaves of a provenance chain.
func chainLeafRecords(chain *ProvenanceChain) map[string]bool {
	records := make(map[string]bool)
	for id, node := range chain.Nodes {
		if node.Kind == "record" {
			records[id] = true
		}
	}
	return records
}

// DerivationOverlap returns the Jaccard similarity between two beliefs' derivation chains,
// considering full ancestry (transitive sources).
func (s *Store) DerivationOverlap(beliefAID, beliefBID string, now time.Time) (float64, error) {
	chainA, err := s.ProvenanceChain(beliefAID, now)
	if err != nil { return 0, err }
	chainB, err := s.ProvenanceChain(beliefBID, now)
	if err != nil { return 0, err }

	recordsA := chainLeafRecords(chainA)
	recordsB := chainLeafRecords(chainB)

	intersect := 0
	for r := range recordsA {
		if recordsB[r] { intersect++ }
	}
	union := len(recordsA) + len(recordsB) - intersect
	if union == 0 { return 0, nil }
	return float64(intersect) / float64(union), nil
}

// EvidenceMatrix returns a pairwise overlap matrix for all beliefs in the store.
// Useful for identifying which beliefs are informationally redundant.
func (s *Store) EvidenceMatrix(now time.Time) ([]string, [][]float64) {
	beliefs := s.AllBeliefs(now)
	ids := make([]string, len(beliefs))
	for i, b := range beliefs { ids[i] = b.BeliefID }
	sort.Strings(ids)

	n := len(ids)
	matrix := make([][]float64, n)
	for i := range matrix {
		matrix[i] = make([]float64, n)
		matrix[i][i] = 1.0 // diagonal
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			overlap, err := s.DerivationOverlap(ids[i], ids[j], now)
			if err != nil { continue }
			matrix[i][j] = overlap
			matrix[j][i] = overlap
		}
	}
	return ids, matrix
}

// RenderEvidenceMatrix renders the matrix as a readable table.
func RenderEvidenceMatrix(ids []string, matrix [][]float64) string {
	if len(ids) == 0 { return "No beliefs in store.\n" }

	// Truncate IDs for display
	labels := make([]string, len(ids))
	for i, id := range ids {
		if len(id) > 10 { labels[i] = id[:10] } else { labels[i] = id }
	}

	var b strings.Builder
	// Header
	fmt.Fprintf(&b, "%-12s", "")
	for _, l := range labels { fmt.Fprintf(&b, "%-12s", l) }
	fmt.Fprintln(&b)

	for i, label := range labels {
		fmt.Fprintf(&b, "%-12s", label)
		for j := range ids {
			v := matrix[i][j]
			if v == 0 { fmt.Fprintf(&b, "%-12s", ".") } else {
				fmt.Fprintf(&b, "%-12s", fmt.Sprintf("%.2f", v))
			}
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

// FindRedundantBeliefs returns pairs of beliefs with high evidential overlap
// that might benefit from merging.
func (s *Store) FindRedundantBeliefs(threshold float64, now time.Time) []RedundancyPair {
	ids, matrix := s.EvidenceMatrix(now)
	var pairs []RedundancyPair
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if matrix[i][j] >= threshold {
				pairs = append(pairs, RedundancyPair{
					BeliefA: ids[i],
					BeliefB: ids[j],
					Overlap: matrix[i][j],
				})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Overlap > pairs[j].Overlap })
	return pairs
}

// RedundancyPair identifies two beliefs with high evidential overlap.
type RedundancyPair struct {
	BeliefA string
	BeliefB string
	Overlap float64
}

// math.Ln2 is not exported so define it here
var _ = math.Ln2
