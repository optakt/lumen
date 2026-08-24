package lumen

import (
	"strings"
	"testing"
	"time"
)

func TestOpaqueFrameParsedCorrectly(t *testing.T) {
	src := `
frame neural-diagnostic
    composition: opaque
    source: "cardiovascular_v3"
    calibration: isotonic
    opacity-reason: "weights not individually addressable"
    decay: exponential halflife: 365d
`
	result, err := ParseFull(src)
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	if len(result.Frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(result.Frames))
	}
	f := result.Frames[0]
	if !f.Opaque {
		t.Error("frame should be marked Opaque")
	}
	if f.Composition != "opaque" {
		t.Errorf("Composition: got %q, want opaque", f.Composition)
	}
	if f.OpaqueSource != "cardiovascular_v3" {
		t.Errorf("OpaqueSource: got %q", f.OpaqueSource)
	}
	if f.Calibration != "isotonic" {
		t.Errorf("Calibration: got %q", f.Calibration)
	}
	t.Logf("opaque frame: source=%s calibration=%s reason=%q",
		f.OpaqueSource, f.Calibration, f.OpaqueReason)
}

func TestOpaqueFrameRegisteredInStore(t *testing.T) {
	src := `
frame opaque-model
    composition: opaque
    source: "risk_model_v2"
    calibration: platt
    opacity-reason: "neural network weights"
    decay: none
`
	s := NewStore()
	now := time.Now()
	if err := LoadFile(src, s, now); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	s.mu.RLock()
	frame, ok := s.frames["opaque-model"]
	s.mu.RUnlock()

	if !ok {
		t.Fatal("frame not registered")
	}
	if !frame.IsOpaque() {
		t.Error("registered frame should be opaque")
	}
	if frame.Calibration != "platt" {
		t.Errorf("Calibration: got %q", frame.Calibration)
	}
	if frame.OpaqueReason != "neural network weights" {
		t.Errorf("OpaqueReason: got %q", frame.OpaqueReason)
	}
	t.Logf("registered: IsOpaque=%v calibration=%s", frame.IsOpaque(), frame.Calibration)
}

func TestOpaqueFrameBlocksEvidenceDecomposition(t *testing.T) {
	// Beliefs in opaque frames should not have evidence blocks processed.
	src := `
frame opaque-model
    composition: opaque
    source: "model_v1"
    calibration: isotonic
    decay: none

record sensor-reading in opaque-model
    "Temperature 37.2C at 09:00."
    at: "2026-01-01T09:00:00Z"

believe fever-likely in opaque-model
    "Patient likely has fever."
    confidence: 0.85
    evidence model-output
        lr: [4.0, 8.0]
        confidence: 0.90
        source: sensor-reading
    from: sensor-reading
`
	s := NewStore()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := LoadFile(src, s, now); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// The belief should exist with the declared confidence (0.85),
	// NOT with the posterior from evidence blocks (which would be higher).
	// Evidence blocks are silently ignored for opaque frames.
	b, err := s.Query("fever-likely", now)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Confidence should be 0.85 (declared), not updated by CredalBayesUpdate
	// (which would push it toward ~0.97 with LR=[4,8]).
	if b.CurrentConfidence > 0.90 {
		t.Errorf("opaque frame evidence update should not apply: confidence=%.3f (expected ~0.85)", b.CurrentConfidence)
	}
	t.Logf("fever-likely confidence (opaque frame, evidence ignored): %.3f", b.CurrentConfidence)
}

func TestBelieveComposedBlockedForOpaque(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{
		Name:        "opaque-f",
		Composition: CompositionOpaque,
		Opaque:      true,
		Calibration: "isotonic",
	})
	now := time.Now()
	_ = s.Assert(&Record{ID: "r1", Content: "Sensor.", Frame: "opaque-f", Timestamp: now})
	_ = s.Believe(&Belief{
		ID:         "b1",
		Content:    "Model output.",
		Confidence: 0.80,
		Frame:      "opaque-f",
		AssertedAt: now,
		Derivation: []string{"r1"},
	})

	// BelieveComposed should fail for opaque frames.
	_, err := s.BelieveComposed(
		&Belief{ID: "b1-composed", Frame: "opaque-f", Content: "composed", Confidence: 0.80, AssertedAt: now},
		0.50,
		[]Evidence{{SourceID: "r1", LikelihoodRatio: 4.0, Confidence: 0.90}},
	)
	if err == nil {
		t.Error("expected BelieveComposed to return error for opaque frame, got nil")
	}
	if !strings.Contains(err.Error(), "opaque") {
		t.Errorf("error should mention 'opaque': %v", err)
	}
	t.Logf("BelieveComposed on opaque frame: %v", err)
}

func TestIsOpaqueViaCompositionString(t *testing.T) {
	// Frame.IsOpaque() should return true when Composition==CompositionOpaque
	// even without the Opaque bool field set.
	f := Frame{Name: "test", Composition: CompositionOpaque}
	if !f.IsOpaque() {
		t.Error("Frame with Composition='opaque' should be opaque")
	}
	f2 := Frame{Name: "test2", Composition: CompositionBayesian}
	if f2.IsOpaque() {
		t.Error("Frame with Composition='bayesian' should not be opaque")
	}
	f3 := Frame{Name: "test3", Opaque: true, Composition: CompositionBayesian}
	if !f3.IsOpaque() {
		t.Error("Frame with Opaque=true should be opaque regardless of Composition")
	}
}

func TestOpaqueExplainMentionsOpacity(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{
		Name:         "neural-dx",
		Composition:  CompositionOpaque,
		Opaque:       true,
		Calibration:  "isotonic",
		OpaqueSource: "cardiac_v3",
		OpaqueReason: "weights not individually addressable",
		Decay:        DecayPolicy{Kind: DecayNone},
	})
	now := time.Now()
	_ = s.Assert(&Record{ID: "r1", Content: "Model output.", Frame: "neural-dx", Timestamp: now})
	_ = s.Believe(&Belief{
		ID:         "b1",
		Content:    "High cardiac risk.",
		Confidence: 0.85,
		Frame:      "neural-dx",
		AssertedAt: now,
		Derivation: []string{"r1"},
	})

	explanation, explainErr := s.Explain("b1", now)
	if explainErr != nil { t.Fatalf("Explain: %v", explainErr) }
	if !strings.Contains(explanation, "opaque") {
		t.Errorf("Explain() should mention opacity:\n%s", explanation)
	}
	if !strings.Contains(explanation, "isotonic") {
		t.Errorf("Explain() should mention calibration method:\n%s", explanation)
	}
	n := 400
	if len(explanation) < n { n = len(explanation) }
	t.Log("\n" + explanation[:n])
}

