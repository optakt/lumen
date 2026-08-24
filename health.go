package lumen

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// HealthScore is a composite epistemic health metric for a belief or store.
// It aggregates multiple signals into a 0–100 score.
type HealthScore struct {
	// Score is the overall health (0 = critically unhealthy, 100 = excellent).
	Score float64
	// Components are the individual signals that contribute to the score.
	Components []HealthComponent
	// Warnings are human-readable issues found.
	Warnings []string
	// Grade is a letter grade: A, B, C, D, F.
	Grade string
}

// HealthComponent is one signal in a health score.
type HealthComponent struct {
	Name        string
	Value       float64  // raw value
	Contribution float64 // weighted contribution to total score (0–100 range portion)
	Weight      float64
	Note        string
}

// BeliefHealth computes the epistemic health of a single belief.
//
// Components:
//   Confidence (30%)  — current confidence (post-decay)
//   Freshness  (20%)  — how recently the belief was asserted or re-asserted
//   Source depth (15%) — having sources is better; deep chains reduce somewhat
//   Provenance clean (25%) — no retracted nodes in chain
//   Consistency (10%) — not in conflict with co-mentioned beliefs
func (s *Store) BeliefHealth(beliefID string, now time.Time) (*HealthScore, error) {
	chain, err := s.ProvenanceChain(beliefID, now)
	if err != nil { return nil, err }

	s.mu.RLock()
	b, ok := s.beliefs[beliefID]
	if !ok { s.mu.RUnlock(); return nil, fmt.Errorf("belief %s not found", beliefID) }
	frame := s.frames[b.Frame]
	conf := b.CurrentConfidence(frame, now)
	elapsed := now.Sub(b.AssertedAt)
	state := b.State
	s.mu.RUnlock()

	hs := &HealthScore{}
	var components []HealthComponent

	// 1. Confidence (30%)
	confScore := conf * 100
	confNote := fmt.Sprintf("current confidence: %.0f%%", conf*100)
	switch state {
	case BeliefSuspect:
		// Reduce score: confidence is real but provenance is untrustworthy pending re-evaluation.
		confScore *= 0.5
		confNote += " [SUSPECT — pending re-evaluation]"
		hs.Warnings = append(hs.Warnings, "Belief is suspect — a source was retracted or revised; re-evaluate.")
	case BeliefSuperseded:
		confScore = 0
		confNote += " [SUPERSEDED — retired by merge]"
		hs.Warnings = append(hs.Warnings, "Belief has been superseded by a merge and should not be relied upon.")
	}
	components = append(components, HealthComponent{"Confidence", conf * 100, confScore * 0.30, 0.30, confNote})

	// 2. Freshness (20%) — score decays as belief ages
	// Full score within 30 days, decays to 50% at 1 year, 0% at 5 years
	ageDays := elapsed.Hours() / 24
	var freshnessScore float64
	switch {
	case ageDays <= 30:
		freshnessScore = 100
	case ageDays <= 365:
		freshnessScore = 50 + 50*(365-ageDays)/335 // linear from 100 to 50
	case ageDays <= 5*365:
		freshnessScore = 50 * math.Exp(-0.5*(ageDays-365)/(5*365))
	default:
		freshnessScore = 10
	}
	if frame.Decay.Kind == DecayNone {
		freshnessScore = 100 // timeless beliefs don't age
	}
	freshnessNote := fmt.Sprintf("asserted %.0f days ago", ageDays)
	if frame.Decay.Kind == DecayNone { freshnessNote += " (timeless frame)" }
	components = append(components, HealthComponent{"Freshness", freshnessScore, freshnessScore * 0.20, 0.20, freshnessNote})

	// 3. Source depth (15%) — having sources is good; extremely deep chains slightly penalized
	depth := chain.MaxDepth
	var depthScore float64
	switch {
	case depth == 0:
		depthScore = 40 // foundational assertion, no sources — not great
		hs.Warnings = append(hs.Warnings, "Belief has no declared sources.")
	case depth == 1:
		depthScore = 100 // direct records
	case depth <= 3:
		depthScore = 85 // reasonable chain
	case depth <= 5:
		depthScore = 70 // long chain — more places for error
	default:
		depthScore = 55 // very deep — significant transitive trust required
	}
	depthNote := fmt.Sprintf("provenance depth: %d, records: %d", depth, chain.TotalRecords)
	components = append(components, HealthComponent{"Source depth", float64(depth), depthScore * 0.15, 0.15, depthNote})

	// 4. Provenance cleanliness (25%)
	provenanceScore := 100.0
	if chain.HasRetracted {
		provenanceScore = 20
		hs.Warnings = append(hs.Warnings, "Provenance chain contains retracted nodes.")
	}
	// Check for unknown (ghost) nodes
	for _, node := range chain.Nodes {
		if node.Kind == "unknown" {
			provenanceScore = math.Min(provenanceScore, 50)
			hs.Warnings = append(hs.Warnings, fmt.Sprintf("Source %s not found in store.", node.ID))
		}
	}
	// Check for foundational records — they are bedrock, not weak links.
	// A belief with a foundational record in its ancestry has deliberate chain termination,
	// which is a positive epistemological signal.
	hasFoundational := false
	for _, node := range chain.Nodes {
		if node.Foundational {
			hasFoundational = true
			break
		}
	}
	// Check for stale derivation sources.
	staleDerivers := s.StaleDerivers(beliefID, now)
	if len(staleDerivers) > 0 {
		// Don't penalize as harshly as retraction — stale derivers are a warning, not a crisis.
		if provenanceScore > 50 {
			provenanceScore = math.Max(50, provenanceScore-20)
		}
		hs.Warnings = append(hs.Warnings, fmt.Sprintf("Source belief(s) stale: %v", staleDerivers))
	}
	provenanceNote := "all sources present"
	if hasFoundational { provenanceNote += "; chain anchored at foundational record" }
	if chain.HasRetracted { provenanceNote = "contains retracted source" }
	if len(staleDerivers) > 0 { provenanceNote += fmt.Sprintf("; %d stale source belief(s)", len(staleDerivers)) }
	if hasFoundational && chain.HasRetracted { provenanceNote += " (foundational record present)" }
	components = append(components, HealthComponent{"Provenance", provenanceScore, provenanceScore * 0.25, 0.25, provenanceNote})

	// 5. Consistency (10%) — check for declared conflicts
	conflicts := s.ConflictScan(now)
	conflictScore := 100.0
	for _, c := range conflicts {
		if c.BeliefA == beliefID || c.BeliefB == beliefID {
			penalty := c.Strength * 40
			conflictScore -= penalty
			hs.Warnings = append(hs.Warnings, fmt.Sprintf("Potential conflict with %s (strength: %.0f%%): %s",
				otherID(c, beliefID), c.Strength*100, c.Explanation))
		}
	}
	conflictScore = math.Max(0, conflictScore)
	conflictNote := "no conflicts detected"
	if len(conflicts) > 0 { conflictNote = fmt.Sprintf("%d conflict(s)", len(conflicts)) }
	components = append(components, HealthComponent{"Consistency", conflictScore, conflictScore * 0.10, 0.10, conflictNote})

	// Total
	total := 0.0
	for _, c := range components {
		total += c.Contribution
	}
	hs.Score = math.Min(100, math.Max(0, total))
	hs.Components = components
	hs.Grade = scoreGrade(hs.Score)

	return hs, nil
}

// StoreHealth computes the aggregate epistemic health of the entire store.
func (s *Store) StoreHealth(now time.Time) *HealthScore {
	beliefs := s.AllBeliefs(now)
	if len(beliefs) == 0 {
		return &HealthScore{Score: 100, Grade: "A", Warnings: []string{"Empty store."}}
	}

	hs := &HealthScore{}
	var components []HealthComponent

	// Aggregate: fraction of active beliefs
	suspect := 0
	for _, b := range beliefs {
		if b.State == BeliefSuspect { suspect++ }
	}
	suspectFrac := float64(suspect) / float64(len(beliefs))
	suspectScore := (1 - suspectFrac) * 100
	components = append(components, HealthComponent{
		"Active beliefs", float64(len(beliefs) - suspect),
		suspectScore * 0.25, 0.25,
		fmt.Sprintf("%d/%d active, %d suspect", len(beliefs)-suspect, len(beliefs), suspect),
	})
	if suspect > 0 {
		hs.Warnings = append(hs.Warnings, fmt.Sprintf("%d suspect belief(s) need resolution.", suspect))
	}

	// Average confidence of active beliefs
	totalConf := 0.0
	activeCount := 0
	for _, b := range beliefs {
		if b.State != BeliefSuspect {
			totalConf += b.CurrentConfidence
			activeCount++
		}
	}
	avgConf := 0.0
	if activeCount > 0 { avgConf = totalConf / float64(activeCount) }
	components = append(components, HealthComponent{
		"Avg confidence", avgConf * 100,
		avgConf * 100 * 0.30, 0.30,
		fmt.Sprintf("%.0f%% average confidence across %d active beliefs", avgConf*100, activeCount),
	})
	if avgConf < 0.5 {
		hs.Warnings = append(hs.Warnings, "Average confidence is low — many beliefs may be stale or weakly supported.")
	}

	// Conflict density
	conflicts := s.ConflictScan(now)
	conflictRate := float64(len(conflicts)) / math.Max(1, float64(len(beliefs)))
	conflictScore := math.Max(0, 100-conflictRate*200) // 0.5 conflicts/belief = 0 score
	components = append(components, HealthComponent{
		"Conflict rate", float64(len(conflicts)),
		conflictScore * 0.20, 0.20,
		fmt.Sprintf("%d conflicts across %d beliefs", len(conflicts), len(beliefs)),
	})

	// Graph connectivity: fraction of beliefs with at least one source
	sourced := 0
	s.mu.RLock()
	for _, b := range s.beliefs {
		if len(b.Derivation) > 0 { sourced++ }
	}
	s.mu.RUnlock()
	sourcedFrac := float64(sourced) / math.Max(1, float64(len(beliefs)))
	components = append(components, HealthComponent{
		"Source coverage", sourcedFrac * 100,
		sourcedFrac * 100 * 0.25, 0.25,
		fmt.Sprintf("%.0f%% of beliefs have declared sources", sourcedFrac*100),
	})
	if sourcedFrac < 0.5 {
		hs.Warnings = append(hs.Warnings, "More than half of beliefs lack declared sources.")
	}

	total := 0.0
	for _, c := range components { total += c.Contribution }
	hs.Score = math.Min(100, math.Max(0, total))
	hs.Components = components
	hs.Grade = scoreGrade(hs.Score)
	return hs
}

// Render returns a human-readable health report.
func (hs *HealthScore) Render(subject string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== Epistemic Health: %s ===\n\n", subject)
	fmt.Fprintf(&b, "Score: %.0f/100  Grade: %s\n\n", hs.Score, hs.Grade)

	fmt.Fprintf(&b, "Components:\n")
	for _, c := range hs.Components {
		fmt.Fprintf(&b, "  %-20s %.0f  (weight=%.0f%%)  %s\n",
			c.Name+":", c.Value, c.Weight*100, c.Note)
	}
	fmt.Fprintln(&b)

	if len(hs.Warnings) > 0 {
		fmt.Fprintf(&b, "Issues:\n")
		for _, w := range hs.Warnings {
			fmt.Fprintf(&b, "  ⚠ %s\n", w)
		}
	} else {
		fmt.Fprintf(&b, "✓ No issues detected\n")
	}
	return b.String()
}

func scoreGrade(score float64) string {
	switch {
	case score >= 90: return "A"
	case score >= 80: return "B"
	case score >= 70: return "C"
	case score >= 60: return "D"
	default:          return "F"
	}
}

func otherID(c Conflict, beliefID string) string {
	if c.BeliefA == beliefID { return c.BeliefB }
	return c.BeliefA
}

// StoreHealthSummary produces a sorted list of belief health scores.
func (s *Store) StoreHealthSummary(now time.Time) []BeliefHealthEntry {
	beliefs := s.AllBeliefs(now)
	entries := make([]BeliefHealthEntry, 0, len(beliefs))
	for _, b := range beliefs {
		hs, err := s.BeliefHealth(b.BeliefID, now)
		if err != nil { continue }
		entries = append(entries, BeliefHealthEntry{
			BeliefID: b.BeliefID,
			Content:  b.Content,
			Score:    hs.Score,
			Grade:    hs.Grade,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Score < entries[j].Score // worst first
	})
	return entries
}

// BeliefHealthEntry is a summary row in a store health report.
type BeliefHealthEntry struct {
	BeliefID string
	Content  string
	Score    float64
	Grade    string
}
