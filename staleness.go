package lumen

import (
	"fmt"
	"time"
)

// StaleThreshold is the confidence below which a source belief is considered stale.
// Frames with on_stale_derivation policy use this as the cutoff.
const StaleThreshold = 0.30

// StaleDerivers returns the IDs of source beliefs (not records — records don't
// decay) whose current confidence has fallen below the stale threshold.
// Used by the on_stale_derivation policy and by BeliefHealth.
func (s *Store) StaleDerivers(beliefID string, now time.Time) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, ok := s.beliefs[beliefID]
	if !ok {
		return nil
	}

	var stale []string
	for _, srcID := range b.Derivation {
		src, ok := s.beliefs[srcID]
		if !ok {
			continue // records don't decay; skip
		}
		srcFrame := s.frames[src.Frame]
		if src.CurrentConfidence(srcFrame, now) < StaleThreshold {
			stale = append(stale, srcID)
		}
	}
	return stale
}

// applyOnStaleDerivation checks the frame's on_stale_derivation policy and
// applies it to the belief if any derivation sources are stale.
//
// "mark_suspect": marks the belief as suspect in-store.
// "fail": returns an error (caller should not use the belief's confidence).
// "retry": returns a hint error that re-assertion is needed (same as fail for now).
//
// The call acquires the store lock briefly — do not call while holding it.
func (s *Store) applyOnStaleDerivation(beliefID string, frame Frame, now time.Time) error {
	if frame.OnStaleDerivation == "" {
		return nil
	}
	stale := s.StaleDerivers(beliefID, now)
	if len(stale) == 0 {
		return nil
	}

	switch frame.OnStaleDerivation {
	case "mark_suspect":
		s.mu.Lock()
		if b, ok := s.beliefs[beliefID]; ok && b.State == BeliefActive {
			b.State = BeliefSuspect
		}
		s.mu.Unlock()
	case "fail":
		return fmt.Errorf("belief %q has stale derivation sources %v and frame policy is 'fail'", beliefID, stale)
	case "retry":
		// Mark suspect so the degraded state is visible; return an error so the
		// caller knows re-assertion is needed. Future queries show BeliefSuspect
		// rather than repeating the error until re-assertion occurs.
		s.mu.Lock()
		if b, ok := s.beliefs[beliefID]; ok && b.State == BeliefActive {
			b.State = BeliefSuspect
		}
		s.mu.Unlock()
		return fmt.Errorf("belief %q has stale derivation sources %v: re-assertion required (on_stale_derivation: retry)", beliefID, stale)
	}
	return nil
}
