package lumen

import (
	"strings"
	"testing"
	"time"
)

func TestSummarize(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f1", Decay: DecayPolicy{Kind: "none"}})
	s.RegisterFrame(Frame{Name: "f2", Decay: DecayPolicy{Kind: "exponential", Halflife: 365 * 24 * time.Hour}})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Assert(&Record{ID: "r1", Frame: "f1", Content: "R1.", Timestamp: t0})
	_ = s.Assert(&Record{ID: "r2", Frame: "f2", Content: "R2.", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "b1", Frame: "f1", Content: "High.", Confidence: 0.90, AssertedAt: t0, Derivation: []string{"r1"}})
	_ = s.Believe(&Belief{ID: "b2", Frame: "f2", Content: "Mid.",  Confidence: 0.60, AssertedAt: t0, Derivation: []string{"r2"}})
	_ = s.Believe(&Belief{ID: "b3", Frame: "f1", Content: "Low.",  Confidence: 0.30, AssertedAt: t0, Derivation: []string{"r1"}})

	sum := s.Summarize(t0)

	if sum.TotalBeliefs != 3 { t.Errorf("total beliefs: want 3, got %d", sum.TotalBeliefs) }
	if sum.ActiveBeliefs != 3 { t.Errorf("active beliefs: want 3, got %d", sum.ActiveBeliefs) }
	if sum.TotalRecords != 2 { t.Errorf("total records: want 2, got %d", sum.TotalRecords) }

	// Average confidence should be ~(0.90+0.60+0.30)/3 = 0.60
	if sum.AvgConfidence < 0.55 || sum.AvgConfidence > 0.65 {
		t.Errorf("avg confidence: want ~0.60, got %.3f", sum.AvgConfidence)
	}

	if len(sum.FrameStats) != 2 { t.Errorf("frame stats: want 2 frames, got %d", len(sum.FrameStats)) }

	// Top belief should be b1 at 90%.
	if len(sum.TopBeliefs) == 0 || sum.TopBeliefs[0].ID != "b1" {
		t.Errorf("top belief should be b1")
	}

	rendered := RenderSummary(sum)
	if !strings.Contains(rendered, "Beliefs:") { t.Error("render should contain 'Beliefs:'") }
	if !strings.Contains(rendered, "By frame:") { t.Error("render should contain 'By frame:'") }
	if !strings.Contains(rendered, "Highest confidence:") { t.Error("render should contain confidence section") }
	t.Logf("\n%s", rendered)
}
