package lumen

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Explain produces a natural language explanation of a belief's current epistemic state.
// It covers: what the belief claims, how confident the store is and why,
// what evidence supports it, how that confidence has changed over time,
// and what would cause it to change.
//
// The goal is prose a human can read — not a data dump, not structured output.
// The EpistemicTrace is the raw material; Explain is the interpretation.
func (s *Store) Explain(beliefID string, now time.Time) (string, error) {
	s.mu.RLock()
	b, ok := s.beliefs[beliefID]
	if !ok {
		s.mu.RUnlock()
		return "", fmt.Errorf("belief %s not found", beliefID)
	}
	frame, ok := s.frames[b.Frame]
	if !ok {
		s.mu.RUnlock()
		return "", fmt.Errorf("frame %s not found", b.Frame)
	}
	currentConf := b.CurrentConfidence(frame, now)
	state := b.State
	elapsed := now.Sub(b.AssertedAt)
	derivation := append([]string{}, b.Derivation...)
	content := b.Content
	frameName := b.Frame
	assertedAt := b.AssertedAt
	contractedBy := b.ContractedBy
	s.mu.RUnlock()

	var w strings.Builder

	// Opening: what is this belief?
	fmt.Fprintf(&w, "**%s**\n\n", content)

	// Confidence and state
	confPct := int(math.Round(currentConf * 100))
	switch {
	case state == BeliefSuperseded:
		// Contracted beliefs have a special explanation.
		if contractedBy != "" {
			fmt.Fprintf(&w, "This belief was **contracted** (soft-deleted) when record `%s` was retracted. "+
				"It survives in the store as `BeliefSuperseded`. "+
				"Recovery is possible via `Recover()` once `%s` is re-asserted.\n\n", contractedBy, contractedBy)
		} else {
			fmt.Fprintf(&w, "This belief has been **retired** (superseded). It is no longer active.\n\n")
		}
	case state == BeliefSuspect:
		fmt.Fprintf(&w, "This belief is currently **suspect** — one or more of its sources has been retracted. Its stated confidence of %d%% should not be trusted until the derivation is reviewed.\n\n", confPct)
	case currentConf >= 0.9:
		fmt.Fprintf(&w, "The store holds this at **%d%% confidence** — high, with strong evidentiary support.\n\n", confPct)
	case currentConf >= 0.7:
		fmt.Fprintf(&w, "The store holds this at **%d%% confidence** — solid, though not without uncertainty.\n\n", confPct)
	case currentConf >= 0.5:
		fmt.Fprintf(&w, "The store holds this at **%d%% confidence** — moderate. The evidence supports it but doesn't strongly establish it.\n\n", confPct)
	default:
		fmt.Fprintf(&w, "The store holds this at **%d%% confidence** — low. The evidence is weak or significantly decayed.\n\n", confPct)
	}

	// Frame and decay
	fmt.Fprintf(&w, "It lives in the **%s** frame", frameName)
	if frame.IsOpaque() {
		// Opaque frames have a different epistemic status: evidence cannot be decomposed.
		calNote := ""
		if frame.Calibration != "" {
			calNote = fmt.Sprintf(" Trust is established through **%s calibration** of the opaque source", frame.Calibration)
			if frame.OpaqueSource != "" {
				calNote += fmt.Sprintf(" (%s)", frame.OpaqueSource)
			}
			calNote += " rather than from explicit evidence records."
		}
		if frame.OpaqueReason != "" {
			fmt.Fprintf(&w, " (**opaque**): %s%s\n\n", frame.OpaqueReason, calNote)
		} else {
			fmt.Fprintf(&w, " (**opaque**): evidence in this frame is not individually addressable.%s\n\n", calNote)
		}
		// Skip decay description for opaque frames — calibration replaces provenance.
		goto afterDecay
	}
	switch frame.Decay.Kind {
	case DecayNone:
		fmt.Fprintf(&w, ", which does not decay — this belief is treated as timeless.\n\n")
	case DecayExponential:
		hl := frame.Decay.Halflife
		hlDays := hl.Hours() / 24
		decayFactor := b.ownDecayFactor(frame, now)
		if decayFactor < 0.999 {
			fmt.Fprintf(&w, " with an exponential decay halflife of %.0f days. After %s, the decay factor is %.2f — so the belief's initial confidence of %d%% has drifted to %d%% due to staleness alone.\n\n",
				hlDays, formatElapsed(elapsed), decayFactor, int(math.Round(b.Confidence*100)), confPct)
		} else {
			fmt.Fprintf(&w, " with an exponential decay halflife of %.0f days. The belief is fresh enough that decay has had negligible effect.\n\n", hlDays)
		}
	case DecayStep:
		fmt.Fprintf(&w, " with a step decay policy — confidence is held constant until a cutoff, then drops.\n\n")
	default:
		fmt.Fprintf(&w, ".\n\n")
	}

	afterDecay:
	// Sources
	if len(derivation) > 0 {
		s.mu.RLock()
		var recordSources, beliefSources []string
		for _, srcID := range derivation {
			if rec, ok := s.records[srcID]; ok {
				desc := fmt.Sprintf("%q", rec.Content)
				if rec.Retracted {
					desc += " [RETRACTED]"
				}
				if rec.Foundational {
					desc += " [FOUNDATIONAL — chain terminates here; this is bedrock]"
				}
				recordSources = append(recordSources, fmt.Sprintf("- Record *%s*: %s", srcID, desc))
			} else if src, ok := s.beliefs[srcID]; ok {
				crossFrameNote := ""
				if src.Frame != frameName {
					// Cross-frame derivation. Check if a bridge src.Frame→frameName is declared.
					// RequiresBridge returns true iff a bridge is registered for that direction.
					bridges := s.Bridges.BridgesFor(src.Frame, frameName)
					if len(bridges) > 0 {
						// Declared bridge: note the loss.
						crossFrameNote = fmt.Sprintf(" [cross-frame via bridge %s; loss: %s]", bridges[0].Name, bridges[0].Loss)
					} else {
						// No bridge declared in this direction. Undeclared cross-frame.
						crossFrameNote = fmt.Sprintf(" [cross-frame from %s — no bridge declared; translation assumptions are implicit]", src.Frame)
					}
				}
				beliefSources = append(beliefSources, fmt.Sprintf("- Belief *%s*: %q%s", srcID, src.Content, crossFrameNote))
			}
		}
		s.mu.RUnlock()

		fmt.Fprintf(&w, "**Sources:**\n")
		for _, rs := range recordSources {
			fmt.Fprintf(&w, "%s\n", rs)
		}
		for _, bs := range beliefSources {
			fmt.Fprintf(&w, "%s\n", bs)
		}
		fmt.Fprintf(&w, "\n")
	} else {
		fmt.Fprintf(&w, "This belief has no declared sources — it is a foundational assertion, not derived from other claims.\n\n")
	}

	// Entity context
	entities := s.Entities.EntitiesForNode(beliefID)
	if len(entities) > 0 {
		fmt.Fprintf(&w, "**Named entities:** %s\n\n", strings.Join(entities, ", "))
		// Co-mentioned beliefs
		co := s.Entities.CoMentioned(beliefID, 1)
		if len(co) > 0 {
			fmt.Fprintf(&w, "Other beliefs in this store share entities with this one")
			if len(co) <= 3 {
				var peers []string
				for _, c := range co {
					peers = append(peers, fmt.Sprintf("*%s*", c.NodeID))
				}
				fmt.Fprintf(&w, ": %s", strings.Join(peers, ", "))
			} else {
				fmt.Fprintf(&w, " (%d others)", len(co))
			}
			fmt.Fprintf(&w, ".\n\n")
		}
	}

	// Suspect / retraction notice
	if state == BeliefSuspect {
		fmt.Fprintf(&w, "**Action required:** this belief derives from at least one retracted source. Run `MinimalContraction` to determine which beliefs can be preserved and which must be removed to restore consistency.\n\n")
	}

	// Graph context: what depends on this belief?
	downstream := s.Graph.ReachableByDerivation(beliefID)
	if len(downstream) > 0 {
		fmt.Fprintf(&w, "**Downstream impact:** %d belief(s) transitively depend on this one", len(downstream))
		if len(downstream) <= 3 {
			var ids []string
			for _, id := range downstream {
				ids = append(ids, fmt.Sprintf("*%s*", id))
			}
			fmt.Fprintf(&w, ": %s", strings.Join(ids, ", "))
		}
		fmt.Fprintf(&w, ". Retracting this belief would suspect all of them.\n\n")
	}

	// Semantic references
	refs := s.Graph.SemanticNeighbors(beliefID)
	if len(refs) > 0 {
		fmt.Fprintf(&w, "**Related (non-inferential):** this belief has semantic reference edges to %d other node(s)", len(refs))
		if len(refs) <= 3 {
			var ids []string
			for _, id := range refs {
				ids = append(ids, fmt.Sprintf("*%s*", id))
			}
			fmt.Fprintf(&w, " — %s", strings.Join(ids, ", "))
		}
		fmt.Fprintf(&w, ".\n\n")
	}

	// Stale derivers: source beliefs whose confidence has fallen below threshold.
	staleDerivers := s.StaleDerivers(beliefID, now)
	if len(staleDerivers) > 0 {
		fmt.Fprintf(&w, "**⚠ Stale source beliefs** (confidence < %.0f%%): %v\n",
			StaleThreshold*100, staleDerivers)
		if frame.OnStaleDerivation != StaleIgnore {
			fmt.Fprintf(&w, "Frame policy %q applies — check source beliefs.\n", frame.OnStaleDerivation)
		}
		fmt.Fprintln(&w)
	}

	// What would change this
	fmt.Fprintf(&w, "**What would change this:** ")
	switch {
	case state == BeliefSuspect:
		fmt.Fprintf(&w, "resolving the retracted source — either by reasserting a corrected record or by applying contraction to remove dependent beliefs.")
	case frame.Decay.Kind == DecayExponential:
		fmt.Fprintf(&w, "new supporting evidence reasserted at current time (resetting the decay clock), or retraction of a source record.")
	default:
		fmt.Fprintf(&w, "retraction of a source record, or explicit revision of the declared confidence.")
	}
	fmt.Fprintf(&w, "\n")

	// Time context
	fmt.Fprintf(&w, "\n*Asserted %s ago, as of %s.*\n", formatElapsed(elapsed), assertedAt.Format("2006-01-02"))


	return w.String(), nil
}

// ownDecayFactor computes the decay factor from the belief's own frame only,
// excluding imported cross-frame decay. Used for explanation purposes.
func (b *Belief) ownDecayFactor(frame Frame, now time.Time) float64 {
	elapsed := now.Sub(b.AssertedAt)
	switch frame.Decay.Kind {
	case DecayExponential:
		if frame.Decay.Halflife <= 0 {
			return 1.0
		}
		halflives := float64(elapsed) / float64(frame.Decay.Halflife)
		return math.Exp(-halflives * math.Ln2)
	case DecayStep:
		if frame.Decay.Halflife > 0 && elapsed > frame.Decay.Halflife {
			return frame.Decay.StepTo
		}
		return 1.0
	default:
		return 1.0
	}
}

func formatElapsed(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	case d < 365*24*time.Hour:
		months := int(d.Hours() / (24 * 30))
		if months == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", months)
	default:
		years := d.Hours() / (24 * 365)
		return fmt.Sprintf("%.1f years", years)
	}
}
