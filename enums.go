package lumen

import (
	"encoding/json"
	"fmt"
)

// DecayKind identifies the type of temporal decay applied to beliefs in a frame.
type DecayKind int

const (
	DecayNone        DecayKind = iota // No decay; confidence is permanent.
	DecayExponential                  // Exponential decay with a halflife.
	DecayStep                         // Step function: full confidence until halflife, then drops.
	DecayLinear                       // Linear decay at a fixed rate per day.
)

var decayKindStrings = map[DecayKind]string{
	DecayNone:        "none",
	DecayExponential: "exponential",
	DecayStep:        "step",
	DecayLinear:      "linear",
}

var decayKindFromString = map[string]DecayKind{
	"none":        DecayNone,
	"":            DecayNone,
	"exponential": DecayExponential,
	"step":        DecayStep,
	"linear":      DecayLinear,
}

func (d DecayKind) String() string {
	if s, ok := decayKindStrings[d]; ok {
		return s
	}
	return fmt.Sprintf("DecayKind(%d)", int(d))
}

func ParseDecayKind(s string) (DecayKind, error) {
	if k, ok := decayKindFromString[s]; ok {
		return k, nil
	}
	return DecayNone, fmt.Errorf("unknown decay kind %q (valid: none, exponential, step)", s)
}

func (d DecayKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *DecayKind) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	k, err := ParseDecayKind(s)
	if err != nil {
		return err
	}
	*d = k
	return nil
}

// CompositionMode identifies how evidence is combined within a frame.
type CompositionMode int

const (
	CompositionBayesian       CompositionMode = iota // Standard Bayesian composition.
	CompositionDempsterShafer                        // Dempster-Shafer with belief/plausibility intervals.
	CompositionOpaque                                // Sources cannot be inspected or decomposed.
)

var compositionModeStrings = map[CompositionMode]string{
	CompositionBayesian:       "bayesian",
	CompositionDempsterShafer: "dempster-shafer",
	CompositionOpaque:         "opaque",
}

var compositionModeFromString = map[string]CompositionMode{
	"bayesian":        CompositionBayesian,
	"":                CompositionBayesian,
	"dempster-shafer": CompositionDempsterShafer,
	"opaque":          CompositionOpaque,
}

func (c CompositionMode) String() string {
	if s, ok := compositionModeStrings[c]; ok {
		return s
	}
	return fmt.Sprintf("CompositionMode(%d)", int(c))
}

func ParseCompositionMode(s string) (CompositionMode, error) {
	if m, ok := compositionModeFromString[s]; ok {
		return m, nil
	}
	return CompositionBayesian, fmt.Errorf("unknown composition mode %q (valid: bayesian, dempster-shafer, opaque)", s)
}

func (c CompositionMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

func (c *CompositionMode) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	m, err := ParseCompositionMode(s)
	if err != nil {
		return err
	}
	*c = m
	return nil
}

// StaleAction identifies what to do when a belief's derivation sources have decayed
// below the staleness threshold.
type StaleAction int

const (
	StaleIgnore      StaleAction = iota // No action; stale sources are tolerated.
	StaleMarkSuspect                    // Mark the belief as suspect.
	StaleRetry                          // Return an error requesting re-assertion.
	StaleFail                           // Return an error; hard failure.
)

var staleActionStrings = map[StaleAction]string{
	StaleIgnore:      "",
	StaleMarkSuspect: "mark_suspect",
	StaleRetry:       "retry",
	StaleFail:        "fail",
}

var staleActionFromString = map[string]StaleAction{
	"":             StaleIgnore,
	"mark_suspect": StaleMarkSuspect,
	"retry":        StaleRetry,
	"fail":         StaleFail,
}

func (s StaleAction) String() string {
	if str, ok := staleActionStrings[s]; ok {
		return str
	}
	return fmt.Sprintf("StaleAction(%d)", int(s))
}

func ParseStaleAction(str string) (StaleAction, error) {
	if a, ok := staleActionFromString[str]; ok {
		return a, nil
	}
	return StaleIgnore, fmt.Errorf("unknown stale action %q (valid: mark_suspect, retry, fail)", str)
}

func (s StaleAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *StaleAction) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	a, err := ParseStaleAction(str)
	if err != nil {
		return err
	}
	*s = a
	return nil
}
