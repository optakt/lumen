package lumen

import (
	"fmt"
	"time"
)

// Recover attempts to restore a contracted belief to active status.
//
// K÷5 (Recovery): if a belief was contracted due to the retraction of
// record R, and R is later re-asserted (or a non-retracted record with the
// same ID is present), the contracted belief can be recovered.
//
// Recovery conditions:
//  1. The belief must be in BeliefSuperseded state.
//  2. The belief's ContractedBy record must currently be non-retracted
//     (it was re-asserted after the contraction, or retraction was reversed).
//  3. All other sources in the belief's Derivation must be present and active.
//
// On success: the belief is moved back to BeliefActive, graphs are
// re-populated, and a version snapshot records the recovery.
// On failure: a descriptive error explains why recovery is not possible.
func (s *Store) Recover(beliefID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.beliefs[beliefID]
	if !ok {
		return fmt.Errorf("belief %q not found", beliefID)
	}
	if b.State != BeliefSuperseded {
		return fmt.Errorf("belief %q is not contracted (state: %s)", beliefID, stateToString(b.State))
	}
	// Merged/retired beliefs share BeliefSuperseded but have no contracting
	// record. Recovery is K÷5 — it reverses contraction, not merging. Without
	// this check a merged belief would be silently resurrected alongside the
	// belief that superseded it.
	if b.ContractedBy == "" {
		return fmt.Errorf("belief %q was retired (merged), not contracted; recovery does not apply", beliefID)
	}

	// Check that the contracting record has been re-asserted.
	if b.ContractedBy != "" {
		rec, ok := s.records[b.ContractedBy]
		if !ok {
			return fmt.Errorf("contracting record %q no longer exists; cannot recover %q", b.ContractedBy, beliefID)
		}
		if rec.Retracted {
			return fmt.Errorf("contracting record %q is still retracted; cannot recover %q until it is re-asserted", b.ContractedBy, beliefID)
		}
	}

	// Check that all derivation sources are present and not superseded.
	for _, srcID := range b.Derivation {
		if rec, ok := s.records[srcID]; ok {
			if rec.Retracted {
				return fmt.Errorf("source record %q is retracted; cannot recover %q", srcID, beliefID)
			}
		} else if src, ok := s.beliefs[srcID]; ok {
			if src.State == BeliefSuperseded {
				return fmt.Errorf("source belief %q is contracted; recover it first, then recover %q", srcID, beliefID)
			}
		} else {
			return fmt.Errorf("source %q no longer exists; cannot recover %q", srcID, beliefID)
		}
	}

	// Snapshot the superseded state before recovery.
	s.versions.Snapshot(b, now, "recovered: record "+b.ContractedBy+" re-asserted")

	// Restore to active.
	b.State = BeliefActive
	b.ContractedBy = ""
	s.invalidateConflicts()
	s.invalidateSearch()

	// Re-populate graph indexes.
	for _, srcID := range b.Derivation {
		s.Graph.AddEdge(Edge{From: srcID, To: b.ID, Kind: EdgeDerives})
		// Re-register dependents.
		if s.dependents[srcID] == nil {
			s.dependents[srcID] = make(map[string]bool)
		}
		s.dependents[srcID][b.ID] = true
	}
	s.Entities.ExtractAndIndex(b.ID, b.Content)
	// The original temporal event survives contraction. Recovery is captured in
	// version history; recording the old assertion again would duplicate it.

	return nil
}

// ContractedBeliefs returns beliefs that were removed by MinimalContraction
// (those with ContractedBy set). These can be recovered if their contracting
// records are re-asserted.
//
// Note: merged/retired beliefs also use BeliefSuperseded state, but have empty
// ContractedBy and are not included here.
func (s *Store) ContractedBeliefs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ids []string
	for id, b := range s.beliefs {
		if b.State == BeliefSuperseded && b.ContractedBy != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// RetiredBeliefs returns beliefs that were superseded by MergeBeliefs or other
// non-contraction operations (ContractedBy is empty).
func (s *Store) RetiredBeliefs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ids []string
	for id, b := range s.beliefs {
		if b.State == BeliefSuperseded && b.ContractedBy == "" {
			ids = append(ids, id)
		}
	}
	return ids
}
