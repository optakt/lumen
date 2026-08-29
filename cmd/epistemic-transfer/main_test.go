package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/optakt/lumen/transfer"
)

func TestLoadResultsIgnoresPartialTrailingLineAndReparsesValidity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.jsonl")
	complete := `{"episode":"e","family":"f","variant":"a","model":"m","run":1,"observations":[{"episode":"e","family":"f","variant":"a","model":"m","run":1,"step":0,"step_id":"s","role":"r","state":{"belief":{"lo":0.4,"hi":0.6},"state":"active","accepted_support":[],"rejected_support":[],"action":"hold"},"protocol_compliant":true,"seeded":false,"raw":"{\"belief\":[0.4,0.6],\"state\":\"active\",\"accepted_support\":[],\"rejected_support\":[],\"node_states\":{},\"historical_belief\":null,\"action\":\"hold\"}"}]}` + "\n"
	if err := os.WriteFile(path, []byte(complete+"\n"+`{"episode":`), 0644); err != nil {
		t.Fatal(err)
	}
	results, err := loadResults(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d", len(results))
	}
	if !results[0].Observations[0].State.Validity["belief"] {
		t.Fatal("raw state was not reparsed with field validity")
	}
}

func TestResultCompleteCompatibility(t *testing.T) {
	legacy := runResult{Observations: []transfer.Observation{{State: transfer.State{}}}}
	if !resultIsComplete(legacy, 1) {
		t.Fatal("legacy result without errors should be complete")
	}
	partial := runResult{Observations: []transfer.Observation{{Error: "failed"}}}
	if resultIsComplete(partial, 1) {
		t.Fatal("partial result reported complete")
	}
	truncated := runResult{Complete: true, Observations: []transfer.Observation{{}}}
	if resultIsComplete(truncated, 2) {
		t.Fatal("truncated result reported complete")
	}
}

func TestMergeFeaturesRejectsCollision(t *testing.T) {
	key := signatureKey{model: "m", run: 1, variant: "a"}
	target := map[signatureKey]transfer.FeatureVector{}
	if err := mergeFeatures(target, key, transfer.FeatureVector{"x": 1}); err != nil {
		t.Fatal(err)
	}
	if err := mergeFeatures(target, key, transfer.FeatureVector{"x": 2}); err == nil {
		t.Fatal("expected duplicate feature error")
	}
}
