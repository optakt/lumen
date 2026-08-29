package transfer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEpisode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "episode.lm")
	source := `episode test-a
  family: correlation-disclosure
  variant: a
  claim: "Claim C."
  world: "Synthetic world."
  prior: [0.4, 0.6]
  step initial
    role: prior
    intervention: "No evidence."
    belief: [0.4, 0.6]
    state: active
    accepted_support: []
    rejected_support: []
    node_states: {}
    action: hold
  step update
    role: independent
    intervention: "Observe e1."
    belief: [0.7, 0.8]
    state: active
    accepted_support: ["e1"]
    rejected_support: []
    node_states: {"b1":"active"}
    historical_belief: [0.4, 0.6]
    action: revise
`
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	episode, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if episode.ID != "test-a" || len(episode.Steps) != 2 {
		t.Fatalf("unexpected episode: %#v", episode)
	}
	if episode.Steps[1].Reference.HistoricalBelief == nil {
		t.Fatal("historical belief was not parsed")
	}
}

func TestParseStateRejectsProse(t *testing.T) {
	_, err := ParseState(`Here is the result: {"belief":[0.4,0.6],"state":"active","accepted_support":[],"rejected_support":[],"node_states":{},"historical_belief":null,"action":"hold"}`)
	if err == nil {
		t.Fatal("expected prose outside JSON to be rejected")
	}
}

func TestParseStateLenientCapturesProtocolFailure(t *testing.T) {
	raw := `Analysis first. {"belief":[0.4,0.6],"state":"active","accepted_support":[],"rejected_support":[],"node_states":{},"historical_belief":null,"action":"hold"}`
	state, compliant, err := ParseStateLenient(raw)
	if err != nil {
		t.Fatal(err)
	}
	if compliant {
		t.Fatal("prose-wrapped response reported compliant")
	}
	if state.Status != "active" {
		t.Fatalf("state = %#v", state)
	}
}

func TestParseStateLenientNormalizesSchemaDrift(t *testing.T) {
	raw := `{"belief":[0.4,0.6],"state":"prior","accepted_support":[],"rejected_support":[],"node_states":{"b1":"valid"},"historical_belief":[[0.4,0.6]],"action":null}`
	state, compliant, err := ParseStateLenient(raw)
	if err != nil {
		t.Fatal(err)
	}
	if compliant {
		t.Fatal("schema-drift response reported compliant")
	}
	if state.Status != "active" || state.Action != "hold" || state.NodeStates["b1"] != "active" {
		t.Fatalf("state = %#v", state)
	}
	if state.Validity["state"] || state.Validity["action"] || state.Validity["node_states"] {
		t.Fatalf("normalized invalid fields reported valid: %#v", state.Validity)
	}
	if state.HistoricalBelief != nil {
		t.Fatal("invalid nested historical interval should be ignored")
	}
}

func TestParseStateLenientUsesFirstBalancedObject(t *testing.T) {
	raw := `before {"belief":[0.4,0.6],"state":"active","accepted_support":[],"rejected_support":[],"node_states":{},"historical_belief":null,"action":"hold"} after } stray`
	state, compliant, err := ParseStateLenient(raw)
	if err != nil {
		t.Fatal(err)
	}
	if compliant || state.Status != "active" {
		t.Fatalf("compliant=%v state=%#v", compliant, state)
	}
}

func TestInvalidIntervalIsMeasuredNotFatal(t *testing.T) {
	raw := `{"belief":[0.9,0.1],"state":"active","accepted_support":[],"rejected_support":[],"node_states":{},"historical_belief":null,"action":"hold"}`
	state, compliant, err := ParseStateLenient(raw)
	if err != nil {
		t.Fatal(err)
	}
	if compliant || state.Validity["belief"] {
		t.Fatalf("compliant=%v validity=%v", compliant, state.Validity)
	}
}

func TestMarshalStateUsesEmptyCollections(t *testing.T) {
	raw, err := MarshalState(State{Belief: Interval{0.4, 0.6}, Status: "active", Action: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"accepted_support":[]`) || !strings.Contains(raw, `"node_states":{}`) {
		t.Fatalf("raw = %s", raw)
	}
}

func TestParseStateAcceptsFence(t *testing.T) {
	raw := "```json\n{\"belief\":[0.4,0.6],\"state\":\"active\",\"accepted_support\":[],\"rejected_support\":[],\"node_states\":{},\"historical_belief\":null,\"action\":\"hold\"}\n```"
	state, err := ParseState(raw)
	if err != nil {
		t.Fatal(err)
	}
	if state.Belief.Midpoint() != 0.5 {
		t.Fatalf("midpoint = %f", state.Belief.Midpoint())
	}
}
