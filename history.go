package lumen

import (
	"fmt"
	"strings"
	"time"
)

// ConfidenceChange records a moment when a belief's effective confidence
// crossed a threshold due to decay, retraction of a dependency, or explicit update.
type ConfidenceChange struct {
	At       time.Time
	BeliefID string
	OldConf  float64
	NewConf  float64
	Reason   string
}

// EpistemicTrace returns a human-readable trace of a belief's history:
// its provenance, what it depends on, and how its current confidence was reached.
func (s *Store) EpistemicTrace(beliefID string, now time.Time) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, ok := s.beliefs[beliefID]
	if !ok {
		return "", fmt.Errorf("belief %s not found", beliefID)
	}
	frame, ok := s.frames[b.Frame]
	if !ok {
		return "", fmt.Errorf("frame %s not found", b.Frame)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Epistemic trace for: %s\n", b.ID))
	sb.WriteString(fmt.Sprintf("Content: %q\n", b.Content))
	sb.WriteString(fmt.Sprintf("Frame: %s (%s composition)\n", b.Frame, frame.Composition))
	sb.WriteString(fmt.Sprintf("Asserted: %s (%.3f confidence)\n",
		b.AssertedAt.Format(time.RFC3339), b.Confidence))

	elapsed := now.Sub(b.AssertedAt)
	current := b.CurrentConfidence(frame, now)
	sb.WriteString(fmt.Sprintf("Now: %s (elapsed: %v)\n", now.Format(time.RFC3339), elapsed.Round(time.Minute)))
	sb.WriteString(fmt.Sprintf("Current confidence: %.4f", current))

	if b.State == BeliefSuspect {
		sb.WriteString(" [SUSPECT — dependency retracted]")
	}
	sb.WriteString("\n")

	// Decay breakdown
	ownPolicy := frame.Decay
	if b.DecayOverride != nil {
		ownPolicy = *b.DecayOverride
	}
	ownDecayed := ownPolicy.ApplyDecay(b.Confidence, elapsed)
	sb.WriteString(fmt.Sprintf("  Own decay (%s, halflife=%v): %.4f → %.4f\n",
		ownPolicy.Kind, ownPolicy.Halflife.Round(time.Hour), b.Confidence, ownDecayed))

	if len(b.ImportedDecay) > 0 {
		importedFactor := MostConservativeDecay(b.ImportedDecay, elapsed)
		sb.WriteString(fmt.Sprintf("  Imported decay (most_conservative from %d foreign frames): factor=%.4f\n",
			len(b.ImportedDecay), importedFactor))
	}

	// Derivation
	if len(b.Derivation) > 0 {
		sb.WriteString("Derivation:\n")
		for _, srcID := range b.Derivation {
			if rec, ok := s.records[srcID]; ok {
				status := "active"
				if rec.Retracted {
					status = fmt.Sprintf("RETRACTED at %s: %s", rec.RetractedAt.Format(time.RFC3339), rec.RetractReason)
				}
				sb.WriteString(fmt.Sprintf("  [record] %s: %q [%s]\n", rec.ID, rec.Content, status))
			} else if src, ok := s.beliefs[srcID]; ok {
				srcCurrent := src.CurrentConfidence(s.frames[src.Frame], now)
				crossFrame := ""
				if src.Frame != b.Frame {
					crossFrame = fmt.Sprintf(" (cross-frame from %s)", src.Frame)
				}
				sb.WriteString(fmt.Sprintf("  [belief] %s: %q (conf=%.4f)%s\n",
					src.ID, src.Content, srcCurrent, crossFrame))
			}
		}
	}

	return sb.String(), nil
}

// WhatChangedMyMind returns beliefs whose confidence dropped by more than threshold
// in the interval [from, to], sampled at samplePoints. This is a simple simulation
// of the epistemic archaeology query from the design document.
func (s *Store) WhatChangedMyMind(beliefID string, from, to time.Time, threshold float64, samples int) ([]ConfidenceChange, error) {
	if samples <= 0 {
		return nil, fmt.Errorf("samples must be positive")
	}
	if to.Before(from) {
		return nil, fmt.Errorf("end time must not precede start time")
	}
	if threshold < 0 {
		return nil, fmt.Errorf("threshold must not be negative")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, ok := s.beliefs[beliefID]
	if !ok {
		return nil, fmt.Errorf("belief %s not found", beliefID)
	}
	frame := s.frames[b.Frame]

	interval := to.Sub(from)
	step := interval / time.Duration(samples)

	var changes []ConfidenceChange
	prev := b.CurrentConfidence(frame, from)

	for i := 1; i <= samples; i++ {
		t := from.Add(step * time.Duration(i))
		curr := b.CurrentConfidence(frame, t)
		drop := prev - curr
		if drop > threshold {
			changes = append(changes, ConfidenceChange{
				At:       t,
				BeliefID: beliefID,
				OldConf:  prev,
				NewConf:  curr,
				Reason:   fmt.Sprintf("decay (%.4f drop over %v)", drop, step),
			})
		}
		prev = curr
	}
	return changes, nil
}
