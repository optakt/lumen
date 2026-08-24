package lumen

import (
	"testing"
	"time"
)

func TestBridgeRegistration(t *testing.T) {
	reg := NewBridgeRegistry()

	b := &Bridge{
		Name:        "empirical-to-philosophical",
		FromFrame:   "empirical",
		ToFrame:     "philosophical",
		Loss:        "collapses(temporal_precision)",
		Method:      "abstraction",
		Verified:    false,
		Assumptions: "Empirical regularities hold in philosophical analysis context",
	}
	if err := reg.Register(b); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Duplicate registration should fail
	if err := reg.Register(b); err == nil {
		t.Error("expected error on duplicate registration")
	}

	// Lookup by name
	found, ok := reg.Lookup("empirical-to-philosophical")
	if !ok || found.Loss != b.Loss {
		t.Errorf("Lookup failed: %v", found)
	}

	// BridgesFor
	bridges := reg.BridgesFor("empirical", "philosophical")
	if len(bridges) != 1 {
		t.Errorf("expected 1 bridge, got %d", len(bridges))
	}

	// RequiresBridge
	if !reg.RequiresBridge("empirical", "philosophical") {
		t.Error("expected bridge required for empirical→philosophical")
	}
	if reg.RequiresBridge("philosophical", "empirical") {
		t.Error("expected no bridge required for reverse direction")
	}
	if reg.RequiresBridge("empirical", "empirical") {
		t.Error("same-frame should never require bridge")
	}
}

func TestBridgeParsing(t *testing.T) {
	src := `
frame empirical
  decay: exponential halflife: 43800h

frame philosophical
  decay: none

bridge empirical-to-philosophical : empirical philosophical
  loss: collapses temporal precision
  method: abstraction
  verified: false
  assumes: "empirical regularities generalize"

record r1 in empirical
  "Test empirical finding"

believe b1 in philosophical
  confidence: 0.65
  sources: r1
  "Philosophical conclusion from empirical finding"
`
	now := time.Now()
	store := NewStore()
	if err := LoadFile(src, store, now); err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	// Bridge should be registered
	_, ok := store.Bridges.Lookup("empirical-to-philosophical")
	if !ok {
		t.Error("bridge not registered in store")
	}

	// Belief should be loaded
	result, err := store.Query("b1", now)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if result.CurrentConfidence < 0.6 {
		t.Errorf("unexpected confidence: %f", result.CurrentConfidence)
	}
}

func TestBridgeLossAccumulation(t *testing.T) {
	// Loss annotations should accumulate across bridge chains
	loss1 := AccumulateLoss("", "collapses(temporal_precision)")
	if loss1 != "collapses(temporal_precision)" {
		t.Errorf("unexpected: %s", loss1)
	}

	loss2 := AccumulateLoss(loss1, "discretizes(confidence)")
	expected := "collapses(temporal_precision); discretizes(confidence)"
	if loss2 != expected {
		t.Errorf("expected %q, got %q", expected, loss2)
	}

	// Deduplication: same loss twice
	loss3 := AccumulateLoss(loss2, "collapses(temporal_precision)")
	if loss3 != loss2 {
		t.Errorf("expected deduplication, got %q", loss3)
	}
}

func TestBridgedBeliefAnnotation(t *testing.T) {
	bb := &BridgedBelief{
		Belief: Belief{ID: "b1", Content: "test", Frame: "philosophical"},
		Crossings: []BridgeCrossing{
			{BridgeName: "empirical-to-philosophical", FromFrame: "empirical", ToFrame: "philosophical", LossCarried: "collapses(temporal_precision)"},
		},
		CumulativeLoss: "collapses(temporal_precision)",
	}

	if !bb.IsTranslated() {
		t.Error("expected IsTranslated = true")
	}

	ann := bb.ProvenanceAnnotation()
	if ann == "" {
		t.Error("expected non-empty provenance annotation")
	}
	t.Logf("Annotation: %s", ann)
}
