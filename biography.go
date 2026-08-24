package lumen

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// BeliefBiography is a complete epistemic life story of a belief:
// how its confidence evolved, what caused each change, which sources
// came and went, and what retractions threatened it.
//
// This is the "killer feature" from the Lumen design: no existing system
// can answer "show me every time you changed your mind about X, and what
// caused each change."
type BeliefBiography struct {
	BeliefID   string
	Content    string
	Frame      string
	AssertedAt time.Time

	// ConfidenceArc is the confidence trajectory across the belief's lifetime.
	ConfidenceArc []ConfidencePoint

	// MindChanges are the moments where confidence shifted beyond the threshold.
	// Each entry explains what triggered the change and why it mattered.
	MindChanges []MindChange

	// DecayTrajectory samples the continuous decay curve (empty for non-decaying frames).
	DecayTrajectory []ConfidencePoint
	// SourceHistory is the full list of source additions and removals.
	SourceHistory []QueryEvent

	// Retractions are records in the provenance chain that were retracted.
	Retractions []QueryEvent

	// BridgeCrossings describes any cross-frame derivation in the ancestry.
	BridgeCrossings []FrameCrossing

	// CurrentHealth is the epistemic health score at query time.
	CurrentHealth *HealthScore

	// Provenance is the full evidential chain, for reference.
	Provenance *ProvenanceChain

	ComputedAt time.Time
}

// ConfidencePoint is one moment in the confidence trajectory.
type ConfidencePoint struct {
	At         time.Time
	Confidence float64
	Label      string // "initial", "revised", "current"
}

// MindChange is a confidence shift that exceeded the threshold — a genuine
// update to the epistemic state, not noise.
type MindChange struct {
	At        time.Time
	FromConf  float64
	ToConf    float64
	Delta     float64
	Reason    string
	Magnitude string // "small", "moderate", "large", "decisive"
	Direction string // "strengthened" or "weakened"
}

// BridgeCrossing records a cross-frame derivation found in the ancestry.
type FrameCrossing struct {
	SourceID    string
	SourceFrame string
	TargetFrame string
	Bridge      *Bridge // nil if no bridge was declared for this crossing
	ConfAtCross float64
}

// WhatChangedMyMind builds a complete epistemic biography of a belief,
// surfacing every recorded mind-change above the confidence threshold.
//
// threshold is the minimum |delta| to qualify as a mind-change (e.g. 0.05 = 5%).
// Pass 0 to include every recorded change.
func (s *Store) EpistemicBiography(beliefID string, threshold float64, now time.Time) (*BeliefBiography, error) {
	// --- Identity ---
	s.mu.RLock()
	b, ok := s.beliefs[beliefID]
	if !ok {
		s.mu.RUnlock()
		return nil, fmt.Errorf("belief %q not found", beliefID)
	}
	frame, _ := s.frames[b.Frame]
	currentConf := b.CurrentConfidence(frame, now)
	bio := &BeliefBiography{
		BeliefID:   b.ID,
		Content:    b.Content,
		Frame:      b.Frame,
		AssertedAt: b.AssertedAt,
		ComputedAt: now,
	}
	// Collect cross-frame sources.
	for _, cf := range b.CrossFrame {
		crossing := FrameCrossing{
			SourceID:    cf.SourceBeliefID,
			SourceFrame: cf.SourceFrame,
			TargetFrame: b.Frame,
			ConfAtCross: cf.ConfidenceAtImport,
		}
		// Look up declared bridge.
		if br, ok := s.Bridges.Lookup(cf.SourceFrame + "-to-" + b.Frame); ok {
			crossing.Bridge = br
		} else if br2, ok2 := s.Bridges.Lookup(cf.SourceFrame + "->" + b.Frame); ok2 {
			crossing.Bridge = br2
		}
		bio.BridgeCrossings = append(bio.BridgeCrossings, crossing)
	}
	s.mu.RUnlock()

	// --- Confidence arc ---
	history := s.versions.History(beliefID)
	arc := []ConfidencePoint{}
	if len(history) > 0 {
		arc = append(arc, ConfidencePoint{
			At:         history[0].AssertedAt,
			Confidence: history[0].Confidence,
			Label:      "initial",
		})
		for i, v := range history {
			var nextConf float64
			var label string
			if i+1 < len(history) {
				nextConf = history[i+1].Confidence
				label = history[i+1].ChangeReason
			} else {
				nextConf = currentConf
				label = "current"
			}
			arc = append(arc, ConfidencePoint{
				At:         v.ChangedAt,
				Confidence: nextConf,
				Label:      label,
			})
		}
	} else {
		// No revision history — single point.
		arc = append(arc, ConfidencePoint{
			At:         bio.AssertedAt,
			Confidence: currentConf,
			Label:      "current (no revisions)",
		})
	}
	bio.ConfidenceArc = arc

	// --- Decay trajectory ---
	// Sample the decayed confidence at regular intervals from AssertedAt to now.
	// This shows the continuous effect of decay even when there are no revisions.
	if frame.Decay.Kind != "" && frame.Decay.Kind != "none" {
		duration := now.Sub(bio.AssertedAt)
		if duration > 0 {
			// Use 10 sample points across the lifetime.
			samples := 10
			step := duration / time.Duration(samples)
			for i := 0; i <= samples; i++ {
				st := bio.AssertedAt.Add(step * time.Duration(i))
				// decayed confidence at sample time, using the belief's initial confidence
				// and the frame's decay policy
				sc := b.CurrentConfidence(frame, st)
				bio.DecayTrajectory = append(bio.DecayTrajectory, ConfidencePoint{
					At:         st,
					Confidence: sc,
					Label:      fmt.Sprintf("t+%s", formatDuration(st.Sub(bio.AssertedAt))),
				})
			}
		}
	}

	// --- Mind changes ---
	for i := 0; i+1 < len(arc); i++ {
		delta := arc[i+1].Confidence - arc[i].Confidence
		if math.Abs(delta) < threshold {
			continue
		}
		direction := "strengthened"
		if delta < 0 {
			direction = "weakened"
		}
		magnitude := classifyMagnitude(math.Abs(delta))
		bio.MindChanges = append(bio.MindChanges, MindChange{
			At:        arc[i+1].At,
			FromConf:  arc[i].Confidence,
			ToConf:    arc[i+1].Confidence,
			Delta:     delta,
			Reason:    arc[i+1].Label,
			Magnitude: magnitude,
			Direction: direction,
		})
	}

	// --- Source history ---
	srcEvents, err := s.execSourceChanges(beliefID, now)
	if err != nil {
		return nil, fmt.Errorf("source history: %w", err)
	}
	bio.SourceHistory = srcEvents

	// --- Retractions in ancestry ---
	retractionEvents, err := s.execRetractionEvents(beliefID, now)
	if err != nil {
		return nil, fmt.Errorf("retraction events: %w", err)
	}
	bio.Retractions = retractionEvents

	// --- Provenance chain ---
	chain, err := s.ProvenanceChain(beliefID, now)
	if err != nil {
		return nil, fmt.Errorf("provenance: %w", err)
	}
	bio.Provenance = chain

	// --- Health score ---
	hs, hsErr := s.BeliefHealth(beliefID, now)
	if hsErr == nil {
		bio.CurrentHealth = hs
	}

	return bio, nil
}

// classifyMagnitude labels a confidence delta by epistemic significance.
func classifyMagnitude(abs float64) string {
	switch {
	case abs >= 0.30:
		return "decisive"   // 30%+ shift: fundamentally revised view
	case abs >= 0.15:
		return "large"      // 15–30%: significant update
	case abs >= 0.07:
		return "moderate"   // 7–15%: meaningful evidence
	default:
		return "small"      // < 7%: incremental adjustment
	}
}

// RenderBiography produces a narrative account of a belief's epistemic life.
func RenderBiography(bio *BeliefBiography) string {
	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "═══ Epistemic Biography: %s ═══\n\n", bio.BeliefID)
	fmt.Fprintf(&b, "  \"%s\"\n", truncateDiff(bio.Content))
	fmt.Fprintf(&b, "  frame: %s  ·  asserted: %s  ·  health: %s (%.0f/100)\n\n",
		bio.Frame,
		bio.AssertedAt.Format("2006-01-02"),
		bio.CurrentHealth.Grade, bio.CurrentHealth.Score)

	// Confidence arc
	fmt.Fprintln(&b, "Confidence trajectory:")
	for i, pt := range bio.ConfidenceArc {
		marker := "  "
		if i == 0 {
			marker = "▶ "
		} else if i == len(bio.ConfidenceArc)-1 {
			marker = "◀ "
		} else {
			marker = "  "
		}
		fmt.Fprintf(&b, "  %s%s  %.0f%%  %s\n",
			marker, pt.At.Format("2006-01-02"), pt.Confidence*100, pt.Label)
	}
	fmt.Fprintln(&b)

	// Mind changes
	if len(bio.MindChanges) == 0 {
		fmt.Fprintln(&b, "Mind changes: none — confidence has been stable since assertion.")
	} else {
		fmt.Fprintf(&b, "Mind changes (%d):\n", len(bio.MindChanges))
		for _, mc := range bio.MindChanges {
			sign := "+"
			if mc.Delta < 0 { sign = "-" }
			fmt.Fprintf(&b, "  %s  %s %s%.0f pp  (%.0f%% → %.0f%%)  [%s]",
				mc.At.Format("2006-01-02"),
				mc.Direction, sign,
				math.Abs(mc.Delta)*100,
				mc.FromConf*100, mc.ToConf*100,
				mc.Magnitude)
			if mc.Reason != "" && mc.Reason != "current" && mc.Reason != "(current)" {
				fmt.Fprintf(&b, "  · %s", mc.Reason)
			}
			fmt.Fprintln(&b)
		}
	}
	fmt.Fprintln(&b)

	// Decay trajectory
	if len(bio.DecayTrajectory) > 0 {
		first := bio.DecayTrajectory[0]
		last  := bio.DecayTrajectory[len(bio.DecayTrajectory)-1]
		pctDrop := (first.Confidence - last.Confidence) / math.Max(first.Confidence, 1e-9) * 100
		if pctDrop > 0.5 {
			fmt.Fprintf(&b, "Decay: %.0f%% → %.0f%% over %s (%.1f%% total drop due to decay)\n\n",
				first.Confidence*100, last.Confidence*100,
				formatDuration(last.At.Sub(first.At)), pctDrop)
		}
	}

	// Source history
	if len(bio.SourceHistory) > 0 {
		fmt.Fprintf(&b, "Source changes (%d):\n", len(bio.SourceHistory))
		for _, ev := range bio.SourceHistory {
			fmt.Fprintf(&b, "  %s  %s  %s\n",
				ev.At.Format("2006-01-02"), ev.Action, ev.SourceID)
		}
		fmt.Fprintln(&b)
	}

	// Retractions
	if len(bio.Retractions) == 0 {
		fmt.Fprintln(&b, "Retractions: none in current ancestry.")
	} else {
		fmt.Fprintf(&b, "Retractions in ancestry (%d) — these records are poisoned:\n", len(bio.Retractions))
		for _, ev := range bio.Retractions {
			fmt.Fprintf(&b, "  %s  record %s", ev.At.Format("2006-01-02"), ev.RecordID)
			if ev.RetractReason != "" {
				fmt.Fprintf(&b, "  — %q", ev.RetractReason)
			}
			fmt.Fprintln(&b)
		}
	}
	fmt.Fprintln(&b)

	// Bridge crossings
	if len(bio.BridgeCrossings) > 0 {
		fmt.Fprintf(&b, "Frame crossings (%d):\n", len(bio.BridgeCrossings))
		for _, cr := range bio.BridgeCrossings {
			fmt.Fprintf(&b, "  source %s  (%s → %s)  conf at crossing: %.0f%%",
				cr.SourceID, cr.SourceFrame, cr.TargetFrame, cr.ConfAtCross*100)
			if cr.Bridge != nil {
				fmt.Fprintf(&b, "  bridge: %s  loss: %s  verified: %v",
					cr.Bridge.Name, cr.Bridge.Loss, cr.Bridge.Verified)
			} else {
				fmt.Fprint(&b, "  [no bridge declared — crossing is untracked]")
			}
			fmt.Fprintln(&b)
		}
		fmt.Fprintln(&b)
	}

	// Provenance summary
	if bio.Provenance != nil {
		wl := bio.Provenance.WeakestLink()
		fmt.Fprintf(&b, "Provenance: %d nodes", len(bio.Provenance.Nodes))
		if wl != nil {
			fmt.Fprintf(&b, "  ·  weakest link: %s (%.0f%%)", wl.ID, wl.Confidence*100)
		}
		fmt.Fprintln(&b)
	}

	return b.String()
}

func healthGrade(hs *HealthScore) string {
	if hs == nil { return "?" }
	return hs.Grade
}
func healthScore(hs *HealthScore) float64 {
	if hs == nil { return 0 }
	return hs.Score
}

// formatDuration returns a human-readable duration string.
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days >= 365 {
		return fmt.Sprintf("%.1fy", float64(days)/365)
	}
	if days >= 30 {
		return fmt.Sprintf("%dm", days/30)
	}
	return fmt.Sprintf("%dd", days)
}
