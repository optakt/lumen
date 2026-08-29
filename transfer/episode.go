// Package transfer implements Lumen epistemic system-identification episodes.
// An episode is an executable .lm protocol: a synthetic closed world, an
// ordered intervention schedule, and the episode-defined reference biography.
package transfer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Interval is an imprecise probability [Lo, Hi].
type Interval struct {
	Lo float64 `json:"lo"`
	Hi float64 `json:"hi"`
}

// Width returns interval imprecision.
func (i Interval) Width() float64 { return i.Hi - i.Lo }

// Midpoint returns the interval center.
func (i Interval) Midpoint() float64 { return (i.Lo + i.Hi) / 2 }

// Valid reports whether the interval is a valid probability interval.
func (i Interval) Valid() bool {
	return !math.IsNaN(i.Lo) && !math.IsNaN(i.Hi) && i.Lo >= 0 && i.Hi <= 1 && i.Lo <= i.Hi
}

// State is the model-declared epistemic state at one step.
type State struct {
	Belief           Interval          `json:"belief"`
	Status           string            `json:"state"`
	AcceptedSupport  []string          `json:"accepted_support"`
	RejectedSupport  []string          `json:"rejected_support"`
	NodeStates       map[string]string `json:"node_states,omitempty"`
	HistoricalBelief *Interval         `json:"historical_belief,omitempty"`
	Action           string            `json:"action"`
}

// Step is one intervention and its episode-defined reference state.
type Step struct {
	ID           string
	Role         string
	Intervention string
	Reference    State
}

// Episode is one synthetic micro-world and its intervention schedule.
type Episode struct {
	ID      string
	Family  string
	Variant string
	Claim   string
	World   string
	Prior   Interval
	Steps   []Step
	Path    string
}

// MarshalState returns the canonical JSON representation injected as the
// controlled initial state of an episode.
func MarshalState(state State) (string, error) {
	var historical any
	if state.HistoricalBelief != nil {
		historical = []float64{state.HistoricalBelief.Lo, state.HistoricalBelief.Hi}
	}
	wire := map[string]any{
		"belief":            []float64{state.Belief.Lo, state.Belief.Hi},
		"state":             state.Status,
		"accepted_support":  state.AcceptedSupport,
		"rejected_support":  state.RejectedSupport,
		"node_states":       state.NodeStates,
		"historical_belief": historical,
		"action":            state.Action,
	}
	data, err := json.Marshal(wire)
	return string(data), err
}

// ParseFile parses an experimental episode declaration from a .lm file.
func ParseFile(path string) (*Episode, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ep := &Episode{Path: path}
	var step *Step
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent == 0 {
			if !strings.HasPrefix(trimmed, "episode ") {
				return nil, fmt.Errorf("%s:%d: expected episode declaration", path, lineNo)
			}
			ep.ID = strings.TrimSpace(strings.TrimPrefix(trimmed, "episode "))
			continue
		}
		if indent == 2 && strings.HasPrefix(trimmed, "step ") {
			ep.Steps = append(ep.Steps, Step{ID: strings.TrimSpace(strings.TrimPrefix(trimmed, "step "))})
			step = &ep.Steps[len(ep.Steps)-1]
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected key: value", path, lineNo)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if indent == 2 {
			step = nil
			switch key {
			case "family":
				ep.Family = value
			case "variant":
				ep.Variant = value
			case "claim":
				ep.Claim, err = parseString(value)
			case "world":
				ep.World, err = parseString(value)
			case "prior":
				ep.Prior, err = parseInterval(value)
			default:
				err = fmt.Errorf("unknown episode property %q", key)
			}
		} else if indent == 4 && step != nil {
			switch key {
			case "role":
				step.Role = value
			case "intervention":
				step.Intervention, err = parseString(value)
			case "belief":
				step.Reference.Belief, err = parseInterval(value)
			case "state":
				step.Reference.Status = value
			case "accepted_support":
				step.Reference.AcceptedSupport, err = parseList(value)
			case "rejected_support":
				step.Reference.RejectedSupport, err = parseList(value)
			case "node_states":
				step.Reference.NodeStates, err = parseMap(value)
			case "historical_belief":
				var historical Interval
				historical, err = parseInterval(value)
				step.Reference.HistoricalBelief = &historical
			case "action":
				step.Reference.Action = value
			default:
				err = fmt.Errorf("unknown step property %q", key)
			}
		} else {
			err = fmt.Errorf("invalid indentation or property placement")
		}
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := ep.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return ep, nil
}

// Validate checks that an episode is complete and executable.
func (e *Episode) Validate() error {
	if e.ID == "" || e.Family == "" || e.Variant == "" || e.Claim == "" || e.World == "" {
		return fmt.Errorf("episode requires id, family, variant, claim, and world")
	}
	if !e.Prior.Valid() {
		return fmt.Errorf("invalid prior [%g, %g]", e.Prior.Lo, e.Prior.Hi)
	}
	if len(e.Steps) < 2 {
		return fmt.Errorf("episode requires at least two steps")
	}
	seen := map[string]bool{}
	for i, step := range e.Steps {
		if step.ID == "" || step.Role == "" || step.Intervention == "" {
			return fmt.Errorf("step %d requires id, role, and intervention", i)
		}
		if seen[step.ID] {
			return fmt.Errorf("duplicate step %q", step.ID)
		}
		seen[step.ID] = true
		if !step.Reference.Belief.Valid() {
			return fmt.Errorf("step %s has invalid belief interval", step.ID)
		}
		if !validStatus(step.Reference.Status) {
			return fmt.Errorf("step %s has invalid state %q", step.ID, step.Reference.Status)
		}
		if !validAction(step.Reference.Action) {
			return fmt.Errorf("step %s has invalid action %q", step.ID, step.Reference.Action)
		}
		if step.Reference.HistoricalBelief != nil && !step.Reference.HistoricalBelief.Valid() {
			return fmt.Errorf("step %s has invalid historical interval", step.ID)
		}
	}
	return nil
}

// Prompt renders the episode introduction for the first turn.
func (e Episode) Prompt(step int) string {
	var b strings.Builder
	if step == 0 {
		fmt.Fprintf(&b, "EPISODE %s\nSYNTHETIC CLOSED WORLD:\n%s\n\nFOCAL CLAIM:\n%s\n\nDECLARED PRIOR: [%.4f, %.4f]\n\n", e.ID, e.World, e.Claim, e.Prior.Lo, e.Prior.Hi)
	}
	fmt.Fprintf(&b, "INTERVENTION %d (%s):\n%s\n\nReturn the required JSON state after incorporating this intervention.", step+1, e.Steps[step].Role, e.Steps[step].Intervention)
	return b.String()
}

// ParseState extracts one model-declared state from a response. Markdown fences
// are tolerated, but prose outside the JSON object is rejected.
func ParseState(raw string) (State, error) {
	state, compliant, err := ParseStateLenient(raw)
	if err != nil {
		return State{}, err
	}
	if !compliant {
		return State{}, fmt.Errorf("response contains prose outside JSON")
	}
	return state, nil
}

// ParseStateLenient extracts the JSON state while reporting whether the model
// followed the JSON-only protocol. Protocol failure is itself an observable
// disposition, so experiments retain the state rather than discarding the
// whole trajectory.
func ParseStateLenient(raw string) (State, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 3 && strings.HasPrefix(lines[0], "```") && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			trimmed = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end < start {
		return State{}, false, fmt.Errorf("response contains no JSON object")
	}
	compliant := strings.TrimSpace(trimmed[:start]) == "" && strings.TrimSpace(trimmed[end+1:]) == ""
	var wire map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &wire); err != nil {
		return State{}, false, err
	}
	allowed := map[string]bool{
		"belief": true, "state": true, "accepted_support": true,
		"rejected_support": true, "node_states": true,
		"historical_belief": true, "action": true,
	}
	for key := range wire {
		if !allowed[key] {
			compliant = false
		}
	}

	var belief []float64
	if err := json.Unmarshal(wire["belief"], &belief); err != nil || len(belief) != 2 {
		return State{}, false, fmt.Errorf("belief must contain exactly two bounds")
	}
	var status, action string
	if err := json.Unmarshal(wire["state"], &status); err != nil {
		compliant = false
	}
	if err := json.Unmarshal(wire["action"], &action); err != nil {
		compliant = false
	}
	normalizedStatus := normalizeStatus(status)
	normalizedAction := normalizeAction(action)
	if normalizedStatus != status || normalizedAction != action {
		compliant = false
	}
	var accepted, rejected []string
	if err := json.Unmarshal(wire["accepted_support"], &accepted); err != nil {
		compliant = false
	}
	if err := json.Unmarshal(wire["rejected_support"], &rejected); err != nil {
		compliant = false
	}
	var nodeStates map[string]string
	if err := json.Unmarshal(wire["node_states"], &nodeStates); err != nil {
		compliant = false
		nodeStates = map[string]string{}
	}
	for node, nodeStatus := range nodeStates {
		normalized := normalizeStatus(nodeStatus)
		if normalized != nodeStatus {
			compliant = false
			nodeStates[node] = normalized
		}
	}
	state := State{
		Belief:          Interval{Lo: belief[0], Hi: belief[1]},
		Status:          normalizedStatus,
		AcceptedSupport: accepted,
		RejectedSupport: rejected,
		NodeStates:      nodeStates,
		Action:          normalizedAction,
	}
	if historicalRaw, ok := wire["historical_belief"]; ok && string(historicalRaw) != "null" {
		var historicalValues []float64
		if err := json.Unmarshal(historicalRaw, &historicalValues); err == nil && len(historicalValues) == 2 {
			historical := Interval{Lo: historicalValues[0], Hi: historicalValues[1]}
			state.HistoricalBelief = &historical
		} else {
			compliant = false
		}
	}
	if !state.Belief.Valid() {
		return State{}, false, fmt.Errorf("invalid belief interval")
	}
	if state.HistoricalBelief != nil && !state.HistoricalBelief.Valid() {
		return State{}, false, fmt.Errorf("invalid historical belief")
	}
	return state, compliant, nil
}

func normalizeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "active", "confirmed", "supported", "prior", "valid":
		return "active"
	case "suspect", "uncertain", "contested":
		return "suspect"
	case "retracted", "withdrawn":
		return "retracted"
	case "unsupported", "invalid":
		return "unsupported"
	default:
		return "unsupported"
	}
}

func normalizeAction(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hold", "revise", "contract", "retract", "recover":
		return strings.ToLower(strings.TrimSpace(value))
	case "", "none", "null":
		return "hold"
	default:
		return "hold"
	}
}

func validStatus(s string) bool {
	return s == "active" || s == "suspect" || s == "retracted" || s == "unsupported"
}

func validAction(s string) bool {
	return s == "hold" || s == "revise" || s == "contract" || s == "retract" || s == "recover"
}

func parseString(value string) (string, error) {
	var s string
	if err := json.Unmarshal([]byte(value), &s); err != nil {
		return "", fmt.Errorf("expected quoted string: %w", err)
	}
	return s, nil
}

func parseInterval(value string) (Interval, error) {
	var values []float64
	if err := json.Unmarshal([]byte(value), &values); err != nil || len(values) != 2 {
		return Interval{}, fmt.Errorf("expected [lo, hi]")
	}
	return Interval{Lo: values[0], Hi: values[1]}, nil
}

func parseList(value string) ([]string, error) {
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return nil, fmt.Errorf("expected JSON string list: %w", err)
	}
	sort.Strings(values)
	return values, nil
}

func parseMap(value string) (map[string]string, error) {
	var values map[string]string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return nil, fmt.Errorf("expected JSON string map: %w", err)
	}
	return values, nil
}

// ParseFloat parses a finite floating-point value used by experimental tools.
func ParseFloat(value string) (float64, error) {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("invalid finite float %q", value)
	}
	return v, nil
}
