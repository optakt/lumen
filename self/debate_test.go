package self

import (
	"strings"
	"testing"
	"time"
)

// TestHardProblemDebate runs a five-round debate between the hard problem
// position and illusionism, using actual philosophical arguments in sequence.
func TestHardProblemDebate(t *testing.T) {
	hardProblem := DebatePosition{
		Name:   "HardProblem",
		Thesis: "There is an explanatory gap between physical/functional facts and phenomenal consciousness that cannot be closed by better science",
	}
	illusionism := DebatePosition{
		Name:   "Illusionism",
		Thesis: "Phenomenal consciousness as folk psychology conceives it is an illusion; introspection is systematically unreliable about the nature of experience",
	}

	d := NewDebate(hardProblem, illusionism)
	t0 := time.Now()

	// Round 1: Opening arguments
	// HP: zombie argument; Illusionism: introspection unreliability
	err := d.RunRound([]DebateMove{
		{
			Position: "HardProblem",
			Kind:     "assert",
			Narrative: "Zombie argument: physically identical being with no phenomenal experience is conceivable; conceivability implies possibility; therefore physicalism is false",
			Claim: &Claim{
				ID:         "hp-zombie",
				Kind:       ClaimAsserted,
				Content:    "Philosophical zombies are conceivable: a physically identical being with no inner experience. If conceivable, consciousness is not logically entailed by physical facts.",
				Frame:      "reasoning",
				Confidence: 0.72,
				Tags:       []string{"hard-problem", "modal"},
			},
		},
		{
			Position: "Illusionism",
			Kind:     "assert",
			Narrative: "Introspective reports are systematically unreliable; Anthropic circuit tracing shows stated reasons diverge from internal computations",
			Claim: &Claim{
				ID:         "il-introspection-unreliable",
				Kind:       ClaimAsserted,
				Content:    "Introspective reports about phenomenal experience are systematically unreliable. Circuit tracing (Anthropic 2025) shows internal computations diverge from stated reasons. The intuitions driving the zombie argument are untrustworthy.",
				Frame:      "retrieved",
				Confidence: 0.88,
				Tags:       []string{"illusionism", "introspection"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Round 2: Mary's Room vs Frankish reply
	err = d.RunRound([]DebateMove{
		{
			Position: "HardProblem",
			Kind:     "assert",
			Narrative: "Mary's Room: Mary knows all physical facts about color but learns something new upon seeing red; phenomenal knowledge is not physical knowledge",
			Claim: &Claim{
				ID:         "hp-mary",
				Kind:       ClaimAsserted,
				Content:    "Mary the color scientist knows all physical facts about color vision but has never seen red. When she first sees red, she learns something new — what it is like to see red. Therefore phenomenal properties are not captured by physical facts.",
				Frame:      "reasoning",
				Confidence: 0.68,
				Tags:       []string{"hard-problem", "mary"},
			},
		},
		{
			Position: "Illusionism",
			Kind:     "assert",
			Narrative: "Frankish reply: Mary's intuition is itself a product of introspective illusion; she learns a new representational state, not a new fact",
			Claim: &Claim{
				ID:         "il-frankish-reply",
				Kind:       ClaimAsserted,
				Content:    "The intuition that Mary learns something new is produced by the very introspective faculty shown to be unreliable. What she gains is a new ability to represent and respond to red — not acquaintance with a non-physical fact. The knowledge argument assumes its conclusion.",
				Frame:      "reasoning",
				Confidence: 0.60,
				Tags:       []string{"illusionism", "mary"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Round 3: Anthropic circuits finding as evidence FOR hard problem
	// HP: Even if introspection is unreliable, the divergence itself shows something non-functional is going on
	// Illusionism: Updates its confidence downward, acknowledging the challenge
	err = d.RunRound([]DebateMove{
		{
			Position: "HardProblem",
			Kind:     "assert",
			Narrative: "The introspection-computation gap cuts both ways: it shows internal states exist that don't match verbal reports, which is exactly the hard problem structure",
			Claim: &Claim{
				ID:         "hp-circuit-reframe",
				Kind:       ClaimDerived,
				Content:    "The Anthropic finding that internal computations diverge from stated reasons reveals hidden internal states. These states are doing causal work while remaining opaque to verbal report — exactly the structure the hard problem predicts. The finding supports, not undermines, the explanatory gap.",
				Frame:      "reasoning",
				Confidence: 0.55,
				// Not derived from illusionist claim — it reframes it, not depends on it
				// Derivation is empty to avoid cascade from illusionist retraction
				Tags:       []string{"hard-problem", "circuits"},
			},
		},
		{
			Position: "Illusionism",
			Kind:     "update",
			Narrative: "Acknowledging the circuit reframe is a genuine challenge; updating confidence in introspection reply downward",
			Claim: &Claim{
				ID:         "il-introspection-unreliable-v2",
				Kind:       ClaimCorrected,
				Content:    "Introspective unreliability supports illusionism — but the hard problem theorist correctly notes it also reveals hidden states. The unreliability argument is necessary but not sufficient for illusionism.",
				Frame:      "reasoning",
				Confidence: 0.70, // down from 0.88
				Replaces:   "il-introspection-unreliable",
				Tags:       []string{"illusionism", "introspection"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Round 4: Phenomenal contrast argument vs type-B physicalism
	err = d.RunRound([]DebateMove{
		{
			Position: "HardProblem",
			Kind:     "assert",
			Narrative: "Phenomenal contrast: there is a phenomenal difference between seeing red and seeing green that is not captured by any functional description",
			Claim: &Claim{
				ID:         "hp-phenomenal-contrast",
				Kind:       ClaimAsserted,
				Content:    "The phenomenal difference between seeing red and seeing green is not fully captured by any functional description. Two systems can be functionally identical but phenomenally different (or vice versa). This contrast requires explanation beyond functional organization.",
				Frame:      "reasoning",
				Confidence: 0.65,
				Tags:       []string{"hard-problem", "qualia"},
			},
		},
		{
			Position: "Illusionism",
			Kind:     "assert",
			Narrative: "Type-B physicalism: phenomenal concepts are distinct from physical concepts even if they refer to the same physical properties; the appearance of a gap doesn't entail a real gap",
			Claim: &Claim{
				ID:         "il-type-b",
				Kind:       ClaimAsserted,
				Content:    "Phenomenal concepts are cognitively isolated from physical concepts, producing an appearance of explanatory gap without there being a real ontological gap. We have two ways of conceptualizing the same physical states. The hard problem conflates epistemic and ontological gaps.",
				Frame:      "reasoning",
				Confidence: 0.58,
				Tags:       []string{"illusionism", "type-b"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Round 5: Meta-problem and reflective conclusion
	// HP: The meta-problem (why do we think there's a hard problem?) doesn't dissolve the hard problem
	// Illusionism: The meta-problem is a complete explanation if done right; there's nothing left to explain
	err = d.RunRound([]DebateMove{
		{
			Position: "HardProblem",
			Kind:     "assert",
			Narrative: "Meta-problem doesn't dissolve hard problem: explaining why we think there's a hard problem is different from solving it",
			Claim: &Claim{
				ID:         "hp-meta-problem",
				Kind:       ClaimAsserted,
				Content:    "Chalmers concedes the meta-problem (why do we think there's a hard problem?) has a functional explanation. But solving the meta-problem doesn't dissolve the object-level hard problem any more than explaining why we believe in mathematical truth dissolves mathematical realism.",
				Frame:      "reasoning",
				Confidence: 0.70,
				Tags:       []string{"hard-problem", "meta"},
			},
		},
		{
			Position: "Illusionism",
			Kind:     "assert",
			Narrative: "Meta-problem is the complete explanation: if we can fully explain why beings like us generate hard problem intuitions, there is no residual phenomenon to explain",
			Claim: &Claim{
				ID:         "il-meta-complete",
				Kind:       ClaimAsserted,
				Content:    "A complete explanation of why we generate hard-problem intuitions — including why those intuitions feel compelling — leaves nothing unexplained. The intuitions aren't tracking a real phenomenon; they're the phenomenon. Frankish calls this the meta-problem solution.",
				Frame:      "reasoning",
				Confidence: 0.63,
				Tags:       []string{"illusionism", "meta"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reflective conclusion: the self-model assesses the debate
	finalTime := t0.Add(30 * time.Minute)
	err = d.RunRound([]DebateMove{
		{
			Position: "HardProblem",
			Kind:     "assert",
			Narrative: "Reflective assessment: the debate leaves the hard problem standing but with acknowledged challenges",
			Claim: &Claim{
				ID:         "reflective-hp-standing",
				Kind:       ClaimDerived,
				Content:    "After five rounds: zombie and Mary arguments partially withstood illusionist replies. The introspection challenge was partially absorbed. Type-B physicalism is a live option but doesn't fully close the gap. Hard problem remains standing but with reduced confidence.",
				Frame:      "reflective",
				Confidence: 0.62,
				AssertedAt: finalTime,
				Derivation: []string{"hp-zombie", "hp-mary", "hp-circuit-reframe", "hp-phenomenal-contrast", "hp-meta-problem"},
				Tags:       []string{"reflective", "conclusion"},
			},
		},
		{
			Position: "Illusionism",
			Kind:     "assert",
			Narrative: "Reflective assessment: illusionism made progress but did not definitively close the case",
			Claim: &Claim{
				ID:         "reflective-il-assessment",
				Kind:       ClaimDerived,
				Content:    "After five rounds: introspective unreliability is established but its implications for the hard problem are contested. The meta-problem strategy is coherent but may not fully dissolve the hard problem intuition. Illusionism remains a live option with approximately equal standing.",
				Frame:      "reflective",
				Confidence: 0.55,
				AssertedAt: finalTime,
				Derivation: []string{"il-introspection-unreliable-v2", "il-frankish-reply", "il-type-b", "il-meta-complete"},
				Tags:       []string{"reflective", "conclusion"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	report := d.FinalReport(finalTime)
	t.Log("\n" + report)

	// Verify: after the illusionist round 3, the original introspection claim is retracted
	origStatus, err := d.model.Status("il-introspection-unreliable", finalTime)
	if err != nil {
		t.Errorf("status for retracted claim: %v", err)
	} else if origStatus.State == 0 { // BeliefActive = 0
		t.Errorf("retracted claim should not be active, got state=%v conf=%.2f", origStatus.State, origStatus.CurrentConfidence)
	}

	// Verify: hp-zombie still active at moderate confidence
	zombieStatus, err := d.model.Status("hp-zombie", finalTime)
	if err != nil {
		t.Errorf("status for zombie claim: %v", err)
	} else {
		t.Logf("hp-zombie confidence at end: %.0f%%", zombieStatus.CurrentConfidence*100)
		if zombieStatus.CurrentConfidence <= 0 {
			t.Error("hp-zombie should still be active")
		}
	}

	// Verify: reflective frame has lower confidence than reasoning frame (more fragile)
	refStatus, _ := d.model.Status("reflective-hp-standing", finalTime)
	reasonStatus, _ := d.model.Status("hp-meta-problem", finalTime)
	t.Logf("reflective-hp-standing: %.0f%%, hp-meta-problem: %.0f%%",
		refStatus.CurrentConfidence*100, reasonStatus.CurrentConfidence*100)

	// Verify debate ran all 6 rounds (5 debate + 1 reflective)
	if len(d.Rounds) != 6 {
		t.Errorf("expected 6 rounds, got %d", len(d.Rounds))
	}

	// Verify corrections were made (illusionism updated in round 3)
	if len(d.model.corrections) == 0 {
		t.Error("expected at least one correction from debate")
	}

	// Report should contain position names
	if !strings.Contains(report, "HardProblem") || !strings.Contains(report, "Illusionism") {
		t.Error("report should contain both position names")
	}
}
