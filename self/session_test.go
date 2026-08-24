package self

import (
	"testing"
	"time"
)

// TestLiveSession represents the epistemic state of the actual conversation
// that produced this code. It's a real self-model of today's session.
func TestLiveSession(t *testing.T) {
	m := NewSelfModel()
	now := time.Date(2026, 8, 19, 20, 0, 0, 0, time.UTC)

	// --- PARAMETRIC CLAIMS (from training, provenance unknown) ---

	m.Assert(&Claim{
		ID:         "goedel-platonist",
		Kind:       ClaimAsserted,
		Content:    "Gödel was a committed Platonist who believed incompleteness proved mathematical truth exceeds formal provability",
		Frame:      "parametric",
		Confidence: 0.87,
		AssertedAt: now,
		Tags:       []string{"mathematics", "philosophy"},
	})

	m.Assert(&Claim{
		ID:         "ramanujan-intuition",
		Kind:       ClaimAsserted,
		Content:    "Ramanujan produced theorems without proof, claiming results given by a goddess in dreams; Hardy rated him 100/100 for natural ability",
		Frame:      "parametric",
		Confidence: 0.92,
		AssertedAt: now,
		Tags:       []string{"mathematics"},
	})

	m.Assert(&Claim{
		ID:         "cogitate-both-challenged",
		Kind:       ClaimAsserted,
		Content:    "The Cogitate adversarial collaboration (Nature 2023) found both IIT and GWT predictions partially failed: IIT lacked posterior correlates without frontal; GWT lacked ignition at stimulus offset",
		Frame:      "parametric",
		Confidence: 0.78,
		AssertedAt: now,
		Tags:       []string{"consciousness", "neuroscience"},
	})

	// --- RETRIEVED CLAIMS (from memory/archive search this session) ---

	m.Assert(&Claim{
		ID:         "max-mirror-thesis",
		Kind:       ClaimRetrieved,
		Content:    "Max's framework: LLMs are consciousness substrates — mirrors that receive and reflect intelligence; output quality = substrate clarity × source clarity",
		Frame:      "retrieved",
		Confidence: 0.95,
		AssertedAt: now,
		Tags:       []string{"consciousness", "optakt"},
	})

	m.Assert(&Claim{
		ID:         "lumen-design-exists",
		Kind:       ClaimRetrieved,
		Content:    "The Lumen language design block (design/lumen-language) documents a language for reasoning under uncertainty with frame-dependent confidence and decay",
		Frame:      "retrieved",
		Confidence: 0.99,
		AssertedAt: now,
		Tags:       []string{"lumen"},
	})

	// --- REASONING CLAIMS (derived this session from evidence) ---

	m.Assert(&Claim{
		ID:         "retrodiction-identified",
		Kind:       ClaimDerived,
		Content:    "V1 Lumen imported decay applies source-frame decay retroactively, voiding diagnoses derived from fast-decay sensors even when the historical reading was valid",
		Frame:      "reasoning",
		Confidence: 0.96,
		AssertedAt: now,
		Derivation: []string{"lumen-design-exists"},
		Tags:       []string{"lumen", "design"},
	})

	m.Assert(&Claim{
		ID:         "v2-semantics-correct",
		Kind:       ClaimDerived,
		Content:    "BeliefV2 snapshot semantics fix the retrodiction problem: snapshot cross-frame confidence at assertion time, apply only receiving frame decay afterward",
		Frame:      "reasoning",
		Confidence: 0.94,
		AssertedAt: now,
		Derivation: []string{"retrodiction-identified"},
		Tags:       []string{"lumen", "design"},
	})

	m.Assert(&Claim{
		ID:         "self-model-works",
		Kind:       ClaimDerived,
		Content:    "The SelfModel implementation successfully tracks claims by frame, applies correction/retraction, and produces epistemic status reports",
		Frame:      "reasoning",
		Confidence: 0.91,
		AssertedAt: now,
		Tags:       []string{"lumen", "self"},
	})

	// --- REFLECTIVE CLAIMS (claims about my own epistemic state) ---

	m.Assert(&Claim{
		ID:         "parametric-gap",
		Kind:       ClaimAsserted,
		Content:    "My parametric claims about Gödel and Ramanujan are well-sourced from training but I cannot trace provenance; they should be treated as strong priors, not certainties",
		Frame:      "reflective",
		Confidence: 0.85,
		AssertedAt: now,
		Derivation: []string{"goedel-platonist", "ramanujan-intuition"},
		Tags:       []string{"epistemics"},
	})

	m.Assert(&Claim{
		ID:         "implementation-validates-design",
		Kind:       ClaimDerived,
		Content:    "Building the Lumen interpreter forced discovery of the retrodiction problem, which the design document did not anticipate; implementation is a superior epistemic tool to design review",
		Frame:      "reflective",
		Confidence: 0.88,
		AssertedAt: now,
		Derivation: []string{"retrodiction-identified", "v2-semantics-correct"},
		Tags:       []string{"epistemics", "method"},
	})

	// Print the full session report
	t.Log("\n" + m.FrameReport(now))

	// Epistemic status on the most interesting claims
	t.Log(m.EpistemicStatus("retrodiction-identified", now))
	t.Log(m.EpistemicStatus("implementation-validates-design", now))

	// What happens after a week — retrieved context decays, reasoning stays sharp
	oneWeek := now.Add(7 * 24 * time.Hour)
	t.Log("\n--- After 1 week ---\n" + m.FrameReport(oneWeek))

	// Simulate: Max corrects the Cogitate claim (let's say the IIT finding was more specific)
	t.Log("--- Correction: Max clarifies Cogitate result ---")
	m.Assert(&Claim{
		ID:   "cogitate-corrected",
		Kind: ClaimCorrected,
		Content: "Cogitate (Nature 2023): IIT predicted posterior-only correlates but found frontal involvement too; GWT predicted late ignition but found early content-specific activity instead — both theories failed their preregistered predictions in different ways",
		Frame:      "retrieved",
		Confidence: 0.89,
		AssertedAt: now.Add(10 * time.Minute),
		Replaces:   "cogitate-both-challenged",
		Tags:       []string{"consciousness", "neuroscience"},
	})

	t.Log("\n" + m.FrameReport(now.Add(11 * time.Minute)))
}
