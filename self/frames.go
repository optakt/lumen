package self

import (
	"time"

	lumen "github.com/optakt/lumen"
)

// The four epistemic frames of the self-model.
// Each captures a different relationship between the agent and its knowledge.

var ParametricFrame = lumen.Frame{
	Name:        "parametric",
	Composition: lumen.CompositionOpaque,
	// No continuous decay — but hard cutoff at training date.
	// Knowledge here is frozen; there's no mechanism to update it.
	// Provenance is inaccessible: we know the claim but not its source.
	Decay: lumen.DecayPolicy{
		Kind:   lumen.DecayStep,
		StepAt: 0, // conceptual: all parametric knowledge is "old"
		StepTo: 1, // we don't decay it continuously; just mark it differently
	},
	ProvenanceDepth:     0, // no provenance available
	ImportedDecayPolicy: "most_conservative",
}

var RetrievedFrame = lumen.Frame{
	Name:        "retrieved",
	Composition: lumen.CompositionBayesian,
	// Retrieved context decays because memory is imperfect and may be stale.
	// Anchors summarize, history preserves — but both introduce loss.
	Decay: lumen.DecayPolicy{
		Kind:     lumen.DecayExponential,
		Halflife: 7 * 24 * time.Hour, // week-scale; retrieved context ages
	},
	ProvenanceDepth:     3,
	ImportedDecayPolicy: "most_conservative",
}

var ReasoningFrame = lumen.Frame{
	Name:        "reasoning",
	Composition: lumen.CompositionBayesian,
	// Active reasoning doesn't decay within a session — it's fresh.
	// But across sessions, without anchoring, it becomes unreliable.
	Decay: lumen.DecayPolicy{
		Kind: lumen.DecayNone, // within-session reasoning stays sharp
	},
	ProvenanceDepth:     5,
	ImportedDecayPolicy: "most_conservative",
}

var ReflectiveFrame = lumen.Frame{
	Name:        "reflective",
	Composition: lumen.CompositionBayesian,
	// The reflective frame takes outputs of other frames as input.
	// It produces beliefs about beliefs. Moderate decay — reflection
	// is more durable than moment-to-moment reasoning but less permanent
	// than well-sourced retrieved knowledge.
	Decay: lumen.DecayPolicy{
		Kind:     lumen.DecayExponential,
		Halflife: 3 * 24 * time.Hour,
	},
	ProvenanceDepth:     4,
	ImportedDecayPolicy: "most_conservative",
}

// RegisterSelfFrames adds all four self-model frames to a store.
func RegisterSelfFrames(s *lumen.Store) {
	s.RegisterFrame(ParametricFrame)
	s.RegisterFrame(RetrievedFrame)
	s.RegisterFrame(ReasoningFrame)
	s.RegisterFrame(ReflectiveFrame)
}

// ContemporaryFrame is for assessments of the current state of a field or situation —
// distinct from timeless philosophical positions. These should decay on a decade scale
// because "the field is underdetermined" in 2026 may not be true in 2036.
var ContemporaryFrame = lumen.Frame{
	Name:        "contemporary",
	Composition: lumen.CompositionBayesian,
	Decay: lumen.DecayPolicy{
		Kind:     lumen.DecayExponential,
		Halflife: 10 * 365 * 24 * time.Hour, // 10-year halflife
	},
	ProvenanceDepth:     4,
	ImportedDecayPolicy: "most_conservative",
}

// RegisterAllFrames adds all five frames (including contemporary) to a store.
func RegisterAllFrames(s *lumen.Store) {
	s.RegisterFrame(ParametricFrame)
	s.RegisterFrame(RetrievedFrame)
	s.RegisterFrame(ReasoningFrame)
	s.RegisterFrame(ReflectiveFrame)
	s.RegisterFrame(ContemporaryFrame)
}
