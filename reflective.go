package lumen

import (
	"fmt"
	"strings"
	"time"
)

// ReflectiveQuery is a question posed to the reflective frame about
// another belief's epistemic status.
type ReflectiveQuery struct {
	TargetBeliefID string
	Question       string // "is_well_calibrated", "should_update", "what_would_change_this"
}

// ReflectiveAnswer is a belief about another belief.
type ReflectiveAnswer struct {
	TargetBeliefID string
	Question       string
	Answer         string
	MetaConfidence float64 // how confident are we in this answer about the other belief
	Observations   []string
}

// Reflect generates a meta-level analysis of a belief's epistemic status.
// This is the "reflective frame" from the design: beliefs about beliefs.
func (s *Store) Reflect(q ReflectiveQuery, now time.Time) (*ReflectiveAnswer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, ok := s.beliefs[q.TargetBeliefID]
	if !ok {
		return nil, fmt.Errorf("belief %s not found", q.TargetBeliefID)
	}
	frame, ok := s.frames[b.Frame]
	if !ok {
		return nil, fmt.Errorf("frame %s not found", b.Frame)
	}

	current := b.CurrentConfidence(frame, now)
	elapsed := now.Sub(b.AssertedAt)

	switch q.Question {
	case "is_well_calibrated":
		return s.reflectCalibration(b, frame, current, elapsed, now)
	case "should_update":
		return s.reflectUpdate(b, frame, current, elapsed, now)
	case "what_would_change_this":
		return s.reflectChangeConditions(b, frame, current, now)
	default:
		return nil, fmt.Errorf("unknown reflective question: %s", q.Question)
	}
}

func (s *Store) reflectCalibration(b *Belief, frame Frame, current float64, elapsed time.Duration, now time.Time) (*ReflectiveAnswer, error) {
	var obs []string
	metaConf := 0.7 // start with modest meta-confidence

	// Check: is the original confidence plausible given derivation depth?
	depth := len(b.Derivation)
	if depth == 0 {
		obs = append(obs, "no derivation: asserted without evidence (prior or assumption)")
		metaConf -= 0.15
	} else if depth == 1 {
		obs = append(obs, "single source: confidence depends entirely on one record or belief")
	} else {
		obs = append(obs, fmt.Sprintf("multi-source (%d): better grounded, but check for corroboration vs. independence", depth))
	}

	// Check: how much has it decayed?
	decayFraction := 1.0 - (current / b.Confidence)
	if b.Confidence > 0 {
		if decayFraction > 0.5 {
			obs = append(obs, fmt.Sprintf("heavily decayed (%.0f%% of original confidence lost over %v)", decayFraction*100, elapsed.Round(time.Hour)))
			metaConf -= 0.1
		} else if decayFraction > 0.2 {
			obs = append(obs, fmt.Sprintf("moderately decayed (%.0f%% lost over %v)", decayFraction*100, elapsed.Round(time.Hour)))
		} else {
			obs = append(obs, fmt.Sprintf("minimally decayed (%.0f%% lost over %v)", decayFraction*100, elapsed.Round(time.Hour)))
		}
	}

	// Check: cross-frame imported decay — is it dominant?
	if len(b.ImportedDecay) > 0 {
		importedFactor := MostConservativeDecay(b.ImportedDecay, elapsed)
		ownFactor := frame.Decay.ApplyDecay(1.0, elapsed)
		if importedFactor < ownFactor*0.5 {
			obs = append(obs, fmt.Sprintf("WARNING: imported decay (factor=%.3f) dominates own decay (factor=%.3f) — see retrodiction problem in design docs", importedFactor, ownFactor))
			metaConf -= 0.15
		}
	}

	// Check: is it suspect?
	if b.State == BeliefSuspect {
		obs = append(obs, "SUSPECT: one or more dependencies retracted — belief is epistemically void")
		metaConf = 0.0
	}

	// Assess original confidence level
	if b.Confidence > 0.95 {
		obs = append(obs, "original confidence very high (>0.95): verify this isn't overconfidence from weak evidence")
		metaConf -= 0.1
	} else if b.Confidence < 0.5 {
		obs = append(obs, "original confidence below 0.5: this belief was uncertain from the start")
	}

	if metaConf < 0 {
		metaConf = 0
	}

	verdict := "calibration appears reasonable"
	if metaConf < 0.4 {
		verdict = "calibration questionable — should review"
	} else if metaConf > 0.65 {
		verdict = "calibration appears sound"
	}

	return &ReflectiveAnswer{
		TargetBeliefID: b.ID,
		Question:       "is_well_calibrated",
		Answer:         verdict,
		MetaConfidence: metaConf,
		Observations:   obs,
	}, nil
}

func (s *Store) reflectUpdate(b *Belief, frame Frame, current float64, elapsed time.Duration, now time.Time) (*ReflectiveAnswer, error) {
	var obs []string
	shouldUpdate := false

	if current < 0.3 && b.State != BeliefSuspect {
		obs = append(obs, fmt.Sprintf("confidence fallen to %.2f: consider re-measuring or explicitly accepting low confidence", current))
		shouldUpdate = true
	}
	if b.State == BeliefSuspect {
		obs = append(obs, "belief is suspect: must recompute from corrected sources before using")
		shouldUpdate = true
	}
	if elapsed > frame.Decay.Halflife*4 && frame.Decay.Kind != "none" {
		obs = append(obs, fmt.Sprintf("elapsed time (%v) is >4 halflives: evidence base likely stale", elapsed.Round(time.Hour)))
		shouldUpdate = true
	}

	// Check if any source beliefs have themselves decayed significantly
	for _, srcID := range b.Derivation {
		if src, ok := s.beliefs[srcID]; ok {
			srcFrame := s.frames[src.Frame]
			srcCurrent := src.CurrentConfidence(srcFrame, now)
			if srcCurrent < src.Confidence*0.5 {
				obs = append(obs, fmt.Sprintf("source belief %q has decayed to %.2f (from %.2f): this may warrant re-derivation", src.Content, srcCurrent, src.Confidence))
				shouldUpdate = true
			}
		}
	}

	if !shouldUpdate {
		obs = append(obs, fmt.Sprintf("belief appears current (conf=%.3f, elapsed=%v)", current, elapsed.Round(time.Hour)))
	}

	verdict := "no update needed"
	if shouldUpdate {
		verdict = "update recommended"
	}

	return &ReflectiveAnswer{
		TargetBeliefID: b.ID,
		Question:       "should_update",
		Answer:         verdict,
		MetaConfidence: 0.75, // the update check is fairly mechanical, so moderate confidence
		Observations:   obs,
	}, nil
}

func (s *Store) reflectChangeConditions(b *Belief, frame Frame, current float64, now time.Time) (*ReflectiveAnswer, error) {
	var obs []string

	obs = append(obs, fmt.Sprintf("Current confidence: %.3f", current))
	obs = append(obs, "To change this belief, any of the following would help:")

	if b.State == BeliefSuspect {
		obs = append(obs, "  - Resolve the retracted dependency and recompute")
	}

	for _, srcID := range b.Derivation {
		if rec, ok := s.records[srcID]; ok {
			if rec.Retracted {
				obs = append(obs, fmt.Sprintf("  - Restore or replace retracted record %q", rec.ID))
			} else {
				obs = append(obs, fmt.Sprintf("  - Corroborate or contradict record %q (%q)", rec.ID, rec.Content))
			}
		} else if src, ok := s.beliefs[srcID]; ok {
			srcFrame := s.frames[src.Frame]
			srcCurrent := src.CurrentConfidence(srcFrame, now)
			obs = append(obs, fmt.Sprintf("  - Re-assert or update belief %q (currently %.3f)", src.Content, srcCurrent))
		}
	}

	if len(b.ImportedDecay) > 0 {
		obs = append(obs, "  - The most_conservative imported decay is suppressing confidence; consider:")
		obs = append(obs, "    a) Re-asserting cross-frame beliefs with current data")
		obs = append(obs, "    b) Using snapshot semantics for historical evidence (retrodiction fix)")
	}

	obs = append(obs, "  - Assert new corroborating evidence and re-derive with updated confidence")

	return &ReflectiveAnswer{
		TargetBeliefID: b.ID,
		Question:       "what_would_change_this",
		Answer:         "see observations for change conditions",
		MetaConfidence: 0.8, // this is fairly structural
		Observations:   obs,
	}, nil
}

// FormatAnswer formats a ReflectiveAnswer for display.
func FormatAnswer(a *ReflectiveAnswer) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Reflective analysis of %q\n", a.TargetBeliefID))
	sb.WriteString(fmt.Sprintf("Question: %s\n", a.Question))
	sb.WriteString(fmt.Sprintf("Answer: %s (meta-confidence: %.2f)\n", a.Answer, a.MetaConfidence))
	sb.WriteString("Observations:\n")
	for _, o := range a.Observations {
		sb.WriteString("  " + o + "\n")
	}
	return sb.String()
}
