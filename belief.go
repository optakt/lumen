package lumen

import (
	"math"
	"time"
)

// Provenance tracks how deep the derivation chain goes.
type Provenance struct {
	Sources []string // IDs of source records/beliefs
	Depth   int
	Summary string // non-empty when depth exceeds frame's retain limit
}

// DecayPolicy defines how a belief's confidence erodes over time.
type DecayPolicy struct {
	Kind     string        // "exponential", "linear", "step", "none"
	Halflife time.Duration // for exponential
	Rate     float64       // for linear (confidence units per day)
	StepAt   time.Duration // for step: drops to StepTo after this duration
	StepTo   float64
}

// ApplyDecay returns the decayed confidence given elapsed time.
func (d DecayPolicy) ApplyDecay(original float64, elapsed time.Duration) float64 {
	switch d.Kind {
	case "exponential":
		if d.Halflife <= 0 {
			return original
		}
		halves := float64(elapsed) / float64(d.Halflife)
		return original * pow2neg(halves)
	case "linear":
		days := elapsed.Hours() / 24
		decayed := original - d.Rate*days
		if decayed < 0 {
			return 0
		}
		return decayed
	case "step":
		if elapsed >= d.StepAt {
			return d.StepTo
		}
		return original
	case "none", "":
		return original
	}
	return original
}

func pow2neg(x float64) float64 {
	return math.Exp(-x * math.Ln2)
}

// Frame declares the epistemological context for reasoning.
type Frame struct {
	Name                string
	Composition         string // "bayesian", "dempster-shafer", "opaque"
	Decay               DecayPolicy
	ProvenanceDepth     int
	ImportedDecayPolicy string // "most_conservative", "most_permissive", "origin"
	OnStaleDerivation   string // "retry", "mark_suspect", "fail"

	// Opaque frame fields — set when Composition == "opaque".
	// In an opaque frame, evidence cannot be decomposed into individual records
	// with likelihood ratios. The frame's calibration provides aggregate trust.
	Opaque       bool   // true when composition is "opaque"
	OpaqueSource string // model/source identifier (e.g. "cardiovascular_v3")
	OpaqueReason string // why opacity is declared
	Calibration  string // calibration method ("isotonic", "platt", etc.)
}

// IsOpaque reports whether the frame forbids evidence decomposition.
// Bayesian composition and evidence blocks are not available in opaque frames.
func (f Frame) IsOpaque() bool {
	return f.Opaque || f.Composition == "opaque"
}

// MostConservativeDecay returns the decay policy that erodes confidence fastest.
func MostConservativeDecay(policies []DecayPolicy, elapsed time.Duration) float64 {
	// Apply each policy to confidence=1.0 and return the minimum result.
	// The most conservative is the one that decays most aggressively.
	min := 1.0
	for _, p := range policies {
		decayed := p.ApplyDecay(1.0, elapsed)
		if decayed < min {
			min = decayed
		}
	}
	return min
}

// CrossFrameSource records the confidence of a cross-frame source belief at
// the moment it was imported. Fixes the retrodiction problem: foreign-frame
// decay is evaluated once at assertion time rather than ongoing.
//
// Semantics: when belief B in frame F derives from belief A in frame G (G ≠ F),
// we snapshot A's confidence at that moment. That is what was incorporated.
// Whether G's sensors are still fresh now is irrelevant to what was known then.
type CrossFrameSource struct {
	SourceBeliefID     string
	SourceFrame        string
	ConfidenceAtImport float64   // confidence of the source at assertion time
	ImportedAt         time.Time
}

// Record is an immutable fact about a specific moment in time.
// Records never decay. They can be retracted (poisoned) but not modified.
type Record struct {
	ID            string
	Content       string
	Timestamp     time.Time
	Frame         string
	Retracted     bool
	RetractedAt   time.Time
	RetractReason string
	Provenance    Provenance
	// Foundational marks the chain terminus — the epistemological bedrock
	// this system stands on but cannot examine without dissolving itself.
	// Foundational records are not "weak links"; they are axioms.
	Foundational  bool
}

// BeliefState tracks the epistemic status of a belief.
type BeliefState int

const (
	BeliefActive  BeliefState = iota
	BeliefSuspect             // depends on retracted ancestor; pending re-evaluation
	BeliefStale               // decay exceeded threshold
	BeliefSuperseded          // retired by a merge or explicit supersession; terminal
)

// Belief is a living inference about current or general state.
// It decays over time and is derived from records and other beliefs.
type Belief struct {
	ID           string
	Content      string
	Confidence   float64     // confidence at AssertedAt
	AssertedAt   time.Time
	Frame        string
	State        BeliefState
	Provenance   Provenance
	Derivation    []string    // IDs of source records/beliefs
	ContractedBy  string     // if state is BeliefSuperseded, the record ID that triggered contraction
	ImportedDecay []DecayPolicy       // decay policies carried in from foreign frames (legacy)
	DecayOverride *DecayPolicy        // per-belief override; nil means use frame default
	CrossFrame    []CrossFrameSource  // snapshots of cross-frame sources at assertion time
}

// CurrentConfidence returns the confidence adjusted for elapsed decay.
func (b *Belief) CurrentConfidence(frame Frame, now time.Time) float64 {
	// Note: suspect beliefs return their decayed confidence, not zero.
	// Suspect means "pending re-evaluation" (a source was retracted/revised),
	// not "this claim is false." Consumers check b.State to act on suspicion.
	elapsed := now.Sub(b.AssertedAt)

	// Determine which decay policy applies to this belief's own decay
	ownPolicy := frame.Decay
	if b.DecayOverride != nil {
		ownPolicy = *b.DecayOverride
	}
	own := ownPolicy.ApplyDecay(b.Confidence, elapsed)

	if len(b.CrossFrame) > 0 {
		// Snapshot cross-frame semantics (fixes retrodiction problem):
		// At import time, the source's confidence was snapshotted. Only the
		// receiving frame's own decay clock applies going forward.
		// Take the min across cross-frame sources as the most conservative bound.
		for _, cf := range b.CrossFrame {
			// Decay the snapshotted confidence using the receiving frame's policy,
			// measuring elapsed time from when the snapshot was taken.
			sourceElapsed := now.Sub(cf.ImportedAt)
			sourceDecayed := ownPolicy.ApplyDecay(cf.ConfidenceAtImport, sourceElapsed)
			if sourceDecayed < own {
				own = sourceDecayed
			}
		}
	} else if len(b.ImportedDecay) > 0 && frame.ImportedDecayPolicy == "most_conservative" {
		// Legacy path: beliefs without CrossFrame snapshots use ImportedDecay policies.
		// For imported decay: take the most conservative *single* clock (no stacking).
		for _, policy := range b.ImportedDecay {
			importedDecayed := policy.ApplyDecay(b.Confidence, elapsed)
			if importedDecayed < own {
				own = importedDecayed
			}
		}
	}

	if own < 0 {
		return 0
	}
	return own
}

