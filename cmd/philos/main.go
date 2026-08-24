package main

import (
	"fmt"
	"time"

	lumen "github.com/optakt/lumen"
)

func main() {
	s := lumen.NewStore()
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)

	s.RegisterFrame(lumen.Frame{Name: "empirical", Composition: lumen.CompositionBayesian,
		Decay: lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 10 * 365 * 24 * time.Hour},
		ProvenanceDepth: 5, ImportedDecayPolicy: "most_conservative"})
	s.RegisterFrame(lumen.Frame{Name: "philosophical", Composition: lumen.CompositionBayesian,
		Decay: lumen.DecayPolicy{Kind: lumen.DecayNone},
		ProvenanceDepth: 5, ImportedDecayPolicy: "most_conservative"})
	s.RegisterFrame(lumen.Frame{Name: "theoretical", Composition: lumen.CompositionBayesian,
		Decay: lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 20 * 365 * 24 * time.Hour},
		ProvenanceDepth: 4, ImportedDecayPolicy: "most_conservative"})

	type rec struct{ id, frame, content string; ts time.Time }
	for _, r := range []rec{
		{"cogitate-2023", "empirical", "Cogitate (Nature 2023): both IIT posterior-only and GWT late-ignition predictions failed preregistered tests", time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)},
		{"iit-letter-2023", "empirical", "124 researchers signed letter calling IIT pseudoscience (Sept 2023)", time.Date(2023, 9, 15, 0, 0, 0, 0, time.UTC)},
		{"split-brain", "empirical", "Split-brain patients show two separate conscious streams, challenging unity of consciousness", time.Date(1981, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"ncc-posterior", "empirical", "No-report paradigm consistently implicates posterior cortex for visual NCC, not frontal", time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"ncc-frontal", "empirical", "Report-based paradigms implicate frontoparietal networks in conscious access", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"cmd-nejm-2024", "empirical", "Cognitive motor dissociation in 25% of clinically unresponsive patients (NEJM 2024)", time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"psilocybin-2024", "empirical", "Psilocybin induces lasting changes in functional network connectivity correlated with conscious experience (2024)", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"anthropic-circuits-2025", "empirical", "Anthropic (2025): Claude's stated reasoning does not match internal computations; introspective reports unreliable", time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)},
		{"ai-welfare-2024", "empirical", "Fish, Long, Sebo, Chalmers (2024): AI welfare deserves serious consideration given current uncertainty", time.Date(2024, 11, 1, 0, 0, 0, 0, time.UTC)},
		{"zombie-argument", "philosophical", "Chalmers (1996): philosophical zombies are conceivable; consciousness not logically entailed by physical facts", time.Date(1996, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"knowledge-argument", "philosophical", "Jackson (1982): Mary knows all physical facts but learns something new upon seeing red — qualia are non-physical", time.Date(1982, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"illusionism-dennett", "philosophical", "Dennett (1991): phenomenal consciousness is a cognitive illusion; there are no qualia in the philosophically loaded sense", time.Date(1991, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"gwt-baars", "theoretical", "Baars (1988): Global Workspace Theory — consciousness is global broadcast of locally processed information", time.Date(1988, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"iit-tononi", "theoretical", "Tononi (2004): Integrated Information Theory — consciousness = phi, integrated information above decomposability threshold", time.Date(2004, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"orch-or", "theoretical", "Penrose-Hameroff Orch OR (1996): consciousness arises from quantum gravity events in microtubules", time.Date(1996, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"russellian-monism", "philosophical", "Strawson (2006): physical science describes structure/causation but not intrinsic nature; phenomenal properties are intrinsic nature of matter", time.Date(2006, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"hard-problem-chalmers", "philosophical", "Chalmers (1995): even complete functional explanation leaves the hard problem — why there is something it is like to be in those states", time.Date(1995, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"predictive-processing", "theoretical", "Friston (2010): Free Energy Principle — biological systems minimize prediction error; active inference unifies perception, action, and consciousness", time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)},
	} {
		s.Assert(&lumen.Record{ID: r.id, Content: r.content, Timestamp: r.ts, Frame: r.frame})
	}

	type entry struct {
		b        *lumen.Belief
		prior    float64
		evidence []lumen.Evidence
	}

	beliefs := []entry{
		{
			b: &lumen.Belief{ID: "gwt-viable", Content: "Global Workspace Theory correctly identifies the mechanism of conscious access",
				Confidence: 0.52, AssertedAt: now, Frame: "theoretical",
				Derivation: []string{"gwt-baars", "ncc-frontal", "cogitate-2023"}},
			prior: 0.30,
			evidence: []lumen.Evidence{
				{SourceID: "gwt-baars", Confidence: 0.75, LikelihoodRatio: 2.5},
				{SourceID: "ncc-frontal", Confidence: 0.80, LikelihoodRatio: 4.0},
				{SourceID: "cogitate-2023", Confidence: 0.90, LikelihoodRatio: 0.40}, // GWT prediction failed
			},
		},
		{
			b: &lumen.Belief{ID: "iit-viable", Content: "Integrated Information Theory correctly identifies consciousness with phi",
				Confidence: 0.18, AssertedAt: now, Frame: "theoretical",
				Derivation: []string{"iit-tononi", "cogitate-2023", "iit-letter-2023", "ncc-posterior"}},
			prior: 0.25,
			evidence: []lumen.Evidence{
				{SourceID: "iit-tononi", Confidence: 0.70, LikelihoodRatio: 3.0},
				{SourceID: "ncc-posterior", Confidence: 0.85, LikelihoodRatio: 1.8},  // posterior finding supports IIT
				{SourceID: "cogitate-2023", Confidence: 0.90, LikelihoodRatio: 0.25}, // preregistered prediction failed
				{SourceID: "iit-letter-2023", Confidence: 0.75, LikelihoodRatio: 0.30},
			},
		},
		{
			b: &lumen.Belief{ID: "hard-problem-real", Content: "The hard problem is a genuine explanatory gap, not merely a conceptual confusion",
				Confidence: 0.72, AssertedAt: now, Frame: "philosophical",
				Derivation: []string{"hard-problem-chalmers", "zombie-argument", "knowledge-argument", "illusionism-dennett", "anthropic-circuits-2025"}},
			prior: 0.50,
			evidence: []lumen.Evidence{
				{SourceID: "zombie-argument", Confidence: 0.65, LikelihoodRatio: 4.5},
				{SourceID: "knowledge-argument", Confidence: 0.70, LikelihoodRatio: 3.5},
				{SourceID: "illusionism-dennett", Confidence: 0.60, LikelihoodRatio: 0.35}, // illusionism as counter
				{SourceID: "anthropic-circuits-2025", Confidence: 0.90, LikelihoodRatio: 2.0}, // introspective gap supports hard problem
			},
		},
		{
			b: &lumen.Belief{ID: "illusionism-viable", Content: "Illusionism is correct: phenomenal consciousness as folk psychology conceives it does not exist",
				Confidence: 0.15, AssertedAt: now, Frame: "philosophical",
				Derivation: []string{"illusionism-dennett", "hard-problem-chalmers", "zombie-argument"}},
			prior: 0.20,
			evidence: []lumen.Evidence{
				{SourceID: "illusionism-dennett", Confidence: 0.60, LikelihoodRatio: 3.0},
				{SourceID: "hard-problem-chalmers", Confidence: 0.70, LikelihoodRatio: 0.20}, // hard problem counter
				{SourceID: "zombie-argument", Confidence: 0.65, LikelihoodRatio: 0.15},        // zombie argument counter
			},
		},
		{
			b: &lumen.Belief{ID: "russellian-monism-viable", Content: "Russellian monism: consciousness is the intrinsic nature of physical matter",
				Confidence: 0.35, AssertedAt: now, Frame: "philosophical",
				Derivation: []string{"russellian-monism", "hard-problem-chalmers", "zombie-argument"}},
			prior: 0.15,
			evidence: []lumen.Evidence{
				{SourceID: "russellian-monism", Confidence: 0.65, LikelihoodRatio: 5.0},
				{SourceID: "hard-problem-chalmers", Confidence: 0.70, LikelihoodRatio: 3.5},
				{SourceID: "zombie-argument", Confidence: 0.65, LikelihoodRatio: 2.5},
			},
		},
		{
			b: &lumen.Belief{ID: "posterior-ncc-correct", Content: "Neural correlates of consciousness are primarily in posterior cortex, not frontal",
				Confidence: 0.65, AssertedAt: now, Frame: "empirical",
				Derivation: []string{"ncc-posterior", "cogitate-2023", "ncc-frontal"}},
			prior: 0.40,
			evidence: []lumen.Evidence{
				{SourceID: "ncc-posterior", Confidence: 0.85, LikelihoodRatio: 5.0},
				{SourceID: "cogitate-2023", Confidence: 0.90, LikelihoodRatio: 2.0},
				{SourceID: "ncc-frontal", Confidence: 0.80, LikelihoodRatio: 0.50}, // counter-evidence
			},
		},
		{
			b: &lumen.Belief{ID: "covert-consciousness-common", Content: "Covert consciousness in clinically unresponsive patients is common (>20%), reshaping diagnosis and ethics",
				Confidence: 0.80, AssertedAt: now, Frame: "empirical",
				Derivation: []string{"cmd-nejm-2024"}},
			prior: 0.20,
			evidence: []lumen.Evidence{
				{SourceID: "cmd-nejm-2024", Confidence: 0.92, LikelihoodRatio: 18.0},
			},
		},
		{
			b: &lumen.Belief{ID: "ai-could-be-conscious", Content: "Current or near-future AI systems could be morally significant conscious entities",
				Confidence: 0.22, AssertedAt: now, Frame: "theoretical",
				Derivation: []string{"ai-welfare-2024", "anthropic-circuits-2025", "hard-problem-real", "iit-viable"}},
			prior: 0.10,
			evidence: []lumen.Evidence{
				{SourceID: "ai-welfare-2024", Confidence: 0.75, LikelihoodRatio: 3.5},
				{SourceID: "anthropic-circuits-2025", Confidence: 0.85, LikelihoodRatio: 1.5},
				{SourceID: "hard-problem-real", Confidence: 0.72, LikelihoodRatio: 1.8}, // hard problem means hard to rule out
				{SourceID: "iit-viable", Confidence: 0.18, LikelihoodRatio: 4.0},        // if IIT, high-phi AI could qualify
			},
		},
	}

	fmt.Println("=== Lumen: Epistemic Analysis of Consciousness Studies ===")
	fmt.Println()
	fmt.Println("BELIEF COMPOSITION (declared vs computed):")
	fmt.Println()

	for _, e := range beliefs {
		cb, err := s.BelieveComposed(e.b, e.prior, e.evidence)
		if err != nil {
			fmt.Printf("  ERROR %s: %v\n", e.b.ID, err)
			continue
		}
		status := "✓"
		note := ""
		if cb.OverconfidenceWarn {
			status = "⚠ OVER "
			note = fmt.Sprintf("(gap +%.0f%%)", cb.Discrepancy*100)
		} else if cb.UnderconfidenceWarn {
			status = "⚠ UNDER"
			note = fmt.Sprintf("(gap -%.0f%%)", cb.Discrepancy*100)
		}
		fmt.Printf("  %s  %-28s  decl=%.2f  comp=%.2f  %s\n",
			status, e.b.ID, cb.DeclaredConfidence, cb.ComputedConfidence, note)
	}

	// Derived second-order beliefs
	derived := []entry{
		{
			b: &lumen.Belief{ID: "field-underdetermined", Content: "No current consciousness theory is clearly correct; the field is genuinely empirically underdetermined",
				Confidence: 0.82, AssertedAt: now, Frame: "philosophical",
				Derivation: []string{"gwt-viable", "iit-viable", "cogitate-2023", "hard-problem-real"}},
			prior: 0.50,
			evidence: []lumen.Evidence{
				{SourceID: "gwt-viable", Confidence: 0.52, LikelihoodRatio: 2.0},
				{SourceID: "iit-viable", Confidence: 0.18, LikelihoodRatio: 3.0},    // low confidence supports underdetermination
				{SourceID: "cogitate-2023", Confidence: 0.90, LikelihoodRatio: 4.0}, // both theories failed empirical test
				{SourceID: "hard-problem-real", Confidence: 0.72, LikelihoodRatio: 2.5},
			},
		},
		{
			b: &lumen.Belief{ID: "consciousness-ethics-urgent", Content: "The ethics of consciousness — AI, animals, disorders of consciousness — is practically urgent",
				Confidence: 0.88, AssertedAt: now, Frame: "philosophical",
				Derivation: []string{"covert-consciousness-common", "ai-could-be-conscious", "ai-welfare-2024"}},
			prior: 0.40,
			evidence: []lumen.Evidence{
				{SourceID: "covert-consciousness-common", Confidence: 0.80, LikelihoodRatio: 6.0},
				{SourceID: "ai-could-be-conscious", Confidence: 0.22, LikelihoodRatio: 3.5},
				{SourceID: "ai-welfare-2024", Confidence: 0.75, LikelihoodRatio: 4.0},
			},
		},
	}
	for _, e := range derived {
		cb, err := s.BelieveComposed(e.b, e.prior, e.evidence)
		if err != nil {
			fmt.Printf("  ERROR %s: %v\n", e.b.ID, err)
			continue
		}
		status := "✓"
		note := ""
		if cb.OverconfidenceWarn {
			status = "⚠ OVER "
			note = fmt.Sprintf("(gap +%.0f%%)", cb.Discrepancy*100)
		} else if cb.UnderconfidenceWarn {
			status = "⚠ UNDER"
			note = fmt.Sprintf("(gap -%.0f%%)", cb.Discrepancy*100)
		}
		fmt.Printf("  %s  %-28s  decl=%.2f  comp=%.2f  %s\n",
			status, e.b.ID, cb.DeclaredConfidence, cb.ComputedConfidence, note)
	}

	// Current confidence ranking
	fmt.Println()
	fmt.Println("CONFIDENCE RANKING (now):")
	results := s.AllBeliefs(now)
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].CurrentConfidence > results[i].CurrentConfidence {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	for _, r := range results {
		bar := ""
		n := int(r.CurrentConfidence * 20)
		for k := 0; k < n; k++ { bar += "█" }
		for k := n; k < 20; k++ { bar += "░" }
		fmt.Printf("  %s  %5.1f%%  [%-12s]  %s\n", bar, r.CurrentConfidence*100, r.Frame, r.BeliefID)
	}

	// 20-year projection
	fmt.Println()
	fmt.Println("CONFIDENCE IN 20 YEARS (2046) — theoretical/empirical beliefs decay:")
	future := now.Add(20 * 365 * 24 * time.Hour)
	futureResults := s.AllBeliefs(future)
	for i := 0; i < len(futureResults)-1; i++ {
		for j := i + 1; j < len(futureResults); j++ {
			if futureResults[j].CurrentConfidence > futureResults[i].CurrentConfidence {
				futureResults[i], futureResults[j] = futureResults[j], futureResults[i]
			}
		}
	}
	for _, r := range futureResults {
		delta := r.CurrentConfidence - results[func() int {
			for i, x := range results { if x.BeliefID == r.BeliefID { return i } }
			return 0
		}()].CurrentConfidence
		sign := "+"
		if delta < 0 { sign = "" }
		fmt.Printf("  %5.1f%%  (Δ%s%.1f%%)  [%-12s]  %s\n",
			r.CurrentConfidence*100, sign, delta*100, r.Frame, r.BeliefID)
	}

	// DS conflict: GWT vs IIT on frontal necessity
	fmt.Println()
	fmt.Println("DEMPSTER-SHAFER: GWT vs IIT on frontal cortex necessity")
	gwt := lumen.DempsterShaferMass{SourceID: "gwt", MassTrue: 0.65, MassFalse: 0.15, MassUnknown: 0.20}
	iit := lumen.DempsterShaferMass{SourceID: "iit", MassTrue: 0.10, MassFalse: 0.70, MassUnknown: 0.20}
	bel, pls, K, err := lumen.DempsterShaferCompose(gwt, iit)
	if err != nil {
		fmt.Println(" ", err)
	} else {
		fmt.Printf("  Hypothesis: frontal cortex is necessary for consciousness\n")
		fmt.Printf("  GWT says: mostly yes  (mass T=%.2f F=%.2f ?=%.2f)\n", gwt.MassTrue, gwt.MassFalse, gwt.MassUnknown)
		fmt.Printf("  IIT says: mostly no   (mass T=%.2f F=%.2f ?=%.2f)\n", iit.MassTrue, iit.MassFalse, iit.MassUnknown)
		fmt.Printf("  Combined: belief=%.3f  plausibility=%.3f  conflict K=%.3f\n", bel, pls, K)
		if K > 0.4 {
			fmt.Printf("  ⚠ High conflict K=%.3f — theories make incompatible empirical predictions\n", K)
			fmt.Println("    Cogitate tested exactly this; both failed, consistent with high prior conflict")
		}
	}

	// Epistemic traces for most contested beliefs
	fmt.Println()
	fmt.Println("EPISTEMIC TRACES:")
	for _, id := range []string{"iit-viable", "ai-could-be-conscious", "hard-problem-real"} {
		trace, err := s.EpistemicTrace(id, now)
		if err != nil {
			fmt.Printf("  %s: %v\n", id, err)
			continue
		}
		fmt.Println()
		for _, line := range splitLines(trace) {
			fmt.Println("  " + line)
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
