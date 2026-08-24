package lumen

import (
	"strings"
	"testing"
	"time"
)

func TestExportDot(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: DecayNone}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Assert(&Record{ID: "r1", Frame: "f", Content: "Root record.", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "b1", Frame: "f", Content: "Derived.", Confidence: 0.80, AssertedAt: t0, Derivation: []string{"r1"}})

	dot := s.ExportDot(DefaultDotOptions(t0))

	if !strings.HasPrefix(dot, "digraph lumen {") {
		t.Error("DOT output should start with 'digraph lumen {'")
	}
	if !strings.Contains(dot, `"r1"`) {
		t.Error("DOT output should contain record r1")
	}
	if !strings.Contains(dot, `"b1"`) {
		t.Error("DOT output should contain belief b1")
	}
	if !strings.Contains(dot, `"r1" -> "b1"`) {
		t.Error("DOT output should contain edge r1 → b1")
	}
	t.Logf("DOT (%d chars)", len(dot))
}

func TestDotSuspectBorder(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: DecayNone}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Assert(&Record{ID: "r1", Frame: "f", Content: "Root.", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "b1", Frame: "f", Content: "Derived.", Confidence: 0.80, AssertedAt: t0, Derivation: []string{"r1"}})
	_ = s.Retract("r1", "", t0)

	dot := s.ExportDot(DefaultDotOptions(t0))
	if !strings.Contains(dot, "dashed") {
		t.Error("suspect belief should have dashed border")
	}
}
