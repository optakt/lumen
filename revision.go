package lumen

import (
	"fmt"
	"time"
)

// Revision implements AGM belief revision: updating a record and propagating
// the change forward through dependent beliefs.
//
// AGM revision (K*A) satisfies:
//   K*1 (Closure)      — result is a belief set
//   K*2 (Success)      — new information A is in K*A
//   K*3 (Inclusion)    — K*A ⊆ K+A (revision ⊆ expansion)
//   K*4 (Vacuity)      — if ¬A ∉ K, then K*A = K+A
//   K*5 (Consistency)  — K*A is consistent if A is consistent
//   K*6 (Extensionality) — if A ≡ B, then K*A = K*B
//
// In Lumen's terms: revising record R means replacing its content and
// optionally its confidence signal, then marking all dependent beliefs
// as needing re-evaluation. This is a "gentle" revision — dependent beliefs
// are marked suspect rather than deleted, because the new record may still
// support them (just with updated content). The caller decides whether to
// re-assert each suspect belief.
//
// Contrast with MinimalContraction (ApplyContraction), which *removes* beliefs.
// Revision *preserves* them in suspect state so the asserter can update them.

// RevisionResult describes what changed.
type RevisionResult struct {
	// RevisedRecord is the ID of the updated record.
	RevisedRecord string
	// OldContent is the prior content.
	OldContent string
	// NewContent is the new content.
	NewContent string
	// Suspect is the list of belief IDs now marked suspect.
	// The caller should re-evaluate each and either re-assert or retract.
	Suspect []string
	// Unchanged is the list of belief IDs unaffected (no dependency on the revised record).
	Unchanged []string
}

// Revise updates a record's content and marks all dependent beliefs suspect.
// It does NOT delete dependent beliefs — they remain in the store, flagged
// for re-evaluation. The caller inspects Suspect and decides what to do.
//
// If newConfidence > 0, the dependent beliefs' Confidence fields are also
// updated to reflect the revised evidential weight. Pass 0 to leave belief
// confidences unchanged.
func (s *Store) Revise(recordID, newContent string, newConfidence float64, reason string, now time.Time) (*RevisionResult, error) {
	s.mu.Lock()

	rec, ok := s.records[recordID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("record %s not found", recordID)
	}

	oldContent := rec.Content
	rec.Content = newContent

	// Find all beliefs that transitively depend on this record.
	// Graph.ReachableByDerivation takes g.mu internally; calling under s.mu is safe
	// (lock order store→graph is consistent throughout the codebase).
	reachable := s.Graph.ReachableByDerivation(recordID)

	// Mark each dependent belief suspect and optionally update its confidence.
	for _, bID := range reachable {
		b, ok := s.beliefs[bID]
		if !ok {
			continue
		}
		// Snapshot before marking suspect
		s.versions.Snapshot(b, now, "revised")
		b.State = BeliefSuspect
		// Note: we do not auto-scale belief confidence on revision.
		// The asserter must re-evaluate each suspect belief explicitly via ReAssert.
	}


	// Build unchanged list.
	var unchanged []string
	for id := range s.beliefs {
		isReachable := false
		for _, r := range reachable {
			if r == id {
				isReachable = true
				break
			}
		}
		if !isReachable {
			unchanged = append(unchanged, id)
		}
	}
	s.mu.Unlock()

	// Record the revision event in the temporal graph.
	// The revised record gets a new temporal event at 'now' to reflect its update.
	s.Temporal.Record(recordID+":revised", "record", now, []string{recordID})

	result := &RevisionResult{
		RevisedRecord: recordID,
		OldContent:    oldContent,
		NewContent:    newContent,
		Suspect:       reachable,
		Unchanged:     unchanged,
	}
	return result, nil
}

// ReAssert re-asserts a suspect belief with updated content and confidence,
// clearing its suspect state. Used after Revise to update beliefs that still
// hold under the revised record.
func (s *Store) ReAssert(beliefID, newContent string, newConfidence float64, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.beliefs[beliefID]
	if !ok {
		return fmt.Errorf("belief %s not found", beliefID)
	}
	// Snapshot before mutation
	s.versions.Snapshot(b, now, "re-asserted")
	if newContent != "" {
		b.Content = newContent
	}
	if newConfidence > 0 {
		b.Confidence = newConfidence
	}
	b.State = BeliefActive
	b.AssertedAt = now

	// Refresh cross-frame snapshots: re-assertion is a re-evaluation, so the
	// imported confidence is re-read from the source's current state. Without
	// this, the min() clamp in CurrentConfidence keeps decaying from the
	// original import time and drags the fresh assertion toward zero — the
	// retrodiction problem re-entering through the re-assertion path.
	// A source that no longer exists or is superseded keeps its old snapshot
	// (conservative: we cannot re-verify it).
	for i, cf := range b.CrossFrame {
		src, ok := s.beliefs[cf.SourceBeliefID]
		if !ok || src.State == BeliefSuperseded {
			continue
		}
		srcFrame := s.frames[src.Frame]
		b.CrossFrame[i].ConfidenceAtImport = src.CurrentConfidence(srcFrame, now)
		b.CrossFrame[i].ImportedAt = now
	}
	return nil
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
