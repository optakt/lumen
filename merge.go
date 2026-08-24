package lumen

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// MergeResult holds the outcome of merging two beliefs.
type MergeResult struct {
	// MergedID is the ID of the newly created merged belief.
	MergedID string
	// RetiredIDs are the IDs of beliefs that were superseded.
	RetiredIDs []string
	// CombinedConfidence is the computed confidence of the merged belief.
	CombinedConfidence float64
	// CombinationMethod describes how confidence was computed.
	CombinationMethod string
}

// MergeBeliefs creates a new belief that synthesizes two existing beliefs,
// combining their derivation chains and computing a merged confidence.
//
// The merged belief:
//   - has the provided mergedID and content
//   - derives from the union of both beliefs' derivation chains
//   - has confidence computed by the specified method
//   - is placed in the specified frame (or the frame of beliefA if empty)
//
// The original beliefs are NOT deleted by default — they remain as historical
// record. Pass retire=true to mark them as superseded (retracted with reason
// "merged into mergedID").
//
// Confidence combination methods:
//   "noisy-or"    — P(A∨B) = 1 - (1-pA)*(1-pB): treats beliefs as independent
//                   evidence for the same claim. Result is always ≥ max(pA, pB).
//   "conservative" — min(pA, pB): most conservative, use when sources may conflict.
//   "geometric"   — sqrt(pA * pB): geometric mean, balanced between the two.
//   "average"     — (pA + pB) / 2: arithmetic mean.
//   "bayesian"    — Bayesian update: pA as prior, pB as likelihood ratio update.
//                   Models pB as P(evidence | claim) = pB, P(evidence | ¬claim) = 1-pB.
func (s *Store) MergeBeliefs(beliefAID, beliefBID, mergedID, content, frame, method string, retire bool, now time.Time) (*MergeResult, error) {
	s.mu.RLock()
	bA, okA := s.beliefs[beliefAID]
	bB, okB := s.beliefs[beliefBID]
	if !okA { s.mu.RUnlock(); return nil, fmt.Errorf("belief %s not found", beliefAID) }
	if !okB { s.mu.RUnlock(); return nil, fmt.Errorf("belief %s not found", beliefBID) }
	if _, exists := s.beliefs[mergedID]; exists {
		s.mu.RUnlock()
		return nil, fmt.Errorf("belief %s already exists", mergedID)
	}

	frameObj := s.frames[bA.Frame]
	confA := bA.CurrentConfidence(frameObj, now)
	frameObjB := s.frames[bB.Frame]
	confB := bB.CurrentConfidence(frameObjB, now)

	// Derivation: the merged belief derives from A and B directly.
	// Their sources are reachable transitively. Including both source derivations
	// creates double-counting in path-based queries (provenance, Jaccard overlap).
	derivUnion := []string{beliefAID, beliefBID}

	targetFrame := frame
	if targetFrame == "" { targetFrame = bA.Frame }

	s.mu.RUnlock()

	// Compute merged confidence
	conf, methodDesc := combineConfidence(confA, confB, method)

	// Create merged belief
	merged := &Belief{
		ID:          mergedID,
		Frame:       targetFrame,
		Content:     content,
		Confidence:  conf,
		AssertedAt:  now,
		Derivation:  derivUnion,
		State:       BeliefActive,
	}
	if err := s.Believe(merged); err != nil {
		return nil, fmt.Errorf("create merged belief: %w", err)
	}

	// Optionally retire the source beliefs
	var retired []string
	if retire {
		s.mu.Lock()
		for _, id := range []string{beliefAID, beliefBID} {
			if b, ok := s.beliefs[id]; ok {
				// Snapshot before state change so version history is complete.
				s.versions.Snapshot(b, now, "merged into "+mergedID)
				// Use BeliefSuperseded, not BeliefSuspect: supersession is terminal
				// and should not be re-activated by ReAssert. Content is immutable.
				b.State = BeliefSuperseded
			}
		}
		s.invalidateConflicts()
		s.mu.Unlock()
		retired = []string{beliefAID, beliefBID}
	}

	return &MergeResult{
		MergedID:           mergedID,
		RetiredIDs:         retired,
		CombinedConfidence: conf,
		CombinationMethod:  methodDesc,
	}, nil
}

// combineConfidence computes a merged confidence from two values using the named method.
func combineConfidence(pA, pB float64, method string) (float64, string) {
	switch strings.ToLower(method) {
	case "noisy-or", "noisy_or", "":
		// P(A∨B) = 1 - (1-pA)(1-pB): independent evidence for same claim
		result := 1 - (1-pA)*(1-pB)
		return result, fmt.Sprintf("noisy-or(%.2f, %.2f) = %.2f", pA, pB, result)
	case "conservative", "min":
		result := math.Min(pA, pB)
		return result, fmt.Sprintf("min(%.2f, %.2f) = %.2f", pA, pB, result)
	case "geometric":
		result := math.Sqrt(pA * pB)
		return result, fmt.Sprintf("geometric(%.2f, %.2f) = %.2f", pA, pB, result)
	case "average":
		result := (pA + pB) / 2
		return result, fmt.Sprintf("average(%.2f, %.2f) = %.2f", pA, pB, result)
	case "bayesian":
		// pA = prior, pB as likelihood ratio update
		// P(H|E) = P(E|H)*P(H) / [P(E|H)*P(H) + P(E|¬H)*P(¬H)]
		// Treating pB as P(E|H) and (1-pB) as P(E|¬H)
		num := pB * pA
		denom := pB*pA + (1-pB)*(1-pA)
		if denom == 0 { return pA, "bayesian(degenerate) = prior" }
		result := num / denom
		return result, fmt.Sprintf("bayesian(prior=%.2f, likelihood=%.2f) = %.2f", pA, pB, result)
	default:
		// Unknown method — fallback to noisy-or
		result := 1 - (1-pA)*(1-pB)
		return result, fmt.Sprintf("noisy-or(%.2f, %.2f) = %.2f [unknown method %q]", pA, pB, result, method)
	}
}

// FindMergeCandidates scans the store for pairs of beliefs that express
// similar claims and might benefit from merging.
//
// Heuristics:
//   - Same frame and high entity overlap (co-mention score ≥ threshold)
//   - Similar content length (within 2x of each other)
//   - Both active (not suspect)
//   - Different derivation chains (if identical, they're already redundant in a different way)
func (s *Store) FindMergeCandidates(threshold int, now time.Time) []MergeCandidate {
	s.mu.RLock()
	type snap struct {
		id         string
		frame      string
		confidence float64
		derivation []string
		state      BeliefState
		contentLen int
	}
	var beliefs []snap
	for id, b := range s.beliefs {
		frame := s.frames[b.Frame]
		beliefs = append(beliefs, snap{
			id:         id,
			frame:      b.Frame,
			confidence: b.CurrentConfidence(frame, now),
			derivation: append([]string{}, b.Derivation...),
			state:      b.State,
			contentLen: len(b.Content),
		})
	}
	s.mu.RUnlock()

	// Build a lookup for O(1) belief-snap access by ID.
	beliefByID := make(map[string]int, len(beliefs))
	for i, b := range beliefs {
		beliefByID[b.id] = i
	}

	// Inverted entity → belief-node snapshot: same approach as ConflictScan,
	// avoiding the O(n²) × CoMentionedBetween pattern.
	entityNodes := s.Entities.EntitySnapshot()

	type pairKey [2]string
	// pairShared accumulates shared entities per candidate pair.
	pairShared := make(map[pairKey][]string)
	for entityID, nodes := range entityNodes {
		var bids []string
		for _, nid := range nodes {
			if _, ok := beliefByID[nid]; ok {
				bids = append(bids, nid)
			}
		}
		if len(bids) < 2 { continue }
		for i := 0; i < len(bids); i++ {
			for j := i + 1; j < len(bids); j++ {
				a, b := bids[i], bids[j]
				if a > b { a, b = b, a }
				pairShared[pairKey{a, b}] = append(pairShared[pairKey{a, b}], entityID)
			}
		}
	}

	var candidates []MergeCandidate
	for pk, co := range pairShared {
		if len(co) < threshold { continue }
		ai, aok := beliefByID[pk[0]]
		bi, bok := beliefByID[pk[1]]
		if !aok || !bok { continue }
		a, b := beliefs[ai], beliefs[bi]
		if a.frame != b.frame { continue }
		if a.state != BeliefActive || b.state != BeliefActive { continue }
		if a.contentLen == 0 || b.contentLen == 0 { continue }
		ratio := float64(a.contentLen) / float64(b.contentLen)
		if ratio < 0.5 || ratio > 2.0 { continue }
		if derivationIdentical(a.derivation, b.derivation) { continue }
		candidates = append(candidates, MergeCandidate{
			BeliefA:        a.id,
			BeliefB:        b.id,
			SharedEntities: co,
			Frame:          a.frame,
			ConfidenceA:    a.confidence,
			ConfidenceB:    b.confidence,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i].SharedEntities) > len(candidates[j].SharedEntities)
	})
	return candidates
}

// MergeCandidate identifies a pair of beliefs worth considering for merging.
type MergeCandidate struct {
	BeliefA        string
	BeliefB        string
	SharedEntities []string
	Frame          string
	ConfidenceA    float64
	ConfidenceB    float64
}

func derivationIdentical(a, b []string) bool {
	if len(a) != len(b) { return false }
	ac := append([]string{}, a...)
	bc := append([]string{}, b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] { return false }
	}
	return true
}
