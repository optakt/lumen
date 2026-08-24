package lumen

import (
	"strings"
	"testing"
	"time"
)

func TestExplainBasic(t *testing.T) {
	s := NewStore()
	now := time.Now()
	f := Frame{Name: "empirical", Decay: DecayPolicy{Kind: "exponential", Halflife: 30 * 24 * time.Hour}}
	s.RegisterFrame(f)

	s.Assert(&Record{ID: "cogitate-2023", Frame: "empirical", Content: "The Cogitate Consortium found GWT and IIT predictions unconfirmed.", Timestamp: now.Add(-365 * 24 * time.Hour)})
	s.Believe(&Belief{
		ID: "iit-weakened", Frame: "empirical",
		Content:    "Integrated Information Theory is significantly weakened by the Cogitate adversarial collaboration.",
		Confidence: 0.70, AssertedAt: now.Add(-180 * 24 * time.Hour),
		Derivation: []string{"cogitate-2023"},
	})

	explanation, err := s.Explain("iit-weakened", now)
	if err != nil {
		t.Fatalf("Explain failed: %v", err)
	}

	t.Logf("\n%s", explanation)

	// Basic content checks
	if !strings.Contains(explanation, "Integrated Information Theory") {
		t.Error("explanation should contain belief content")
	}
	if !strings.Contains(explanation, "empirical") {
		t.Error("explanation should mention the frame")
	}
	if !strings.Contains(explanation, "cogitate-2023") {
		t.Error("explanation should mention the source record")
	}
}

func TestExplainSuspect(t *testing.T) {
	s := NewStore()
	now := time.Now()
	f := Frame{Name: "test", Decay: DecayPolicy{Kind: "none"}}
	s.RegisterFrame(f)

	s.Assert(&Record{ID: "r1", Frame: "test", Content: "Flawed experiment", Timestamp: now})
	s.Believe(&Belief{ID: "b1", Frame: "test", Content: "Conclusion from flawed experiment", Confidence: 0.8, AssertedAt: now, Derivation: []string{"r1"}})
	s.Retract("r1", "methodology error", now)

	explanation, err := s.Explain("b1", now)
	if err != nil {
		t.Fatalf("Explain failed: %v", err)
	}

	t.Logf("\n%s", explanation)

	if !strings.Contains(strings.ToLower(explanation), "suspect") {
		t.Error("explanation should mention suspect state")
	}
}

func TestExplainTimeless(t *testing.T) {
	s := NewStore()
	now := time.Now()
	f := Frame{Name: "philosophical", Decay: DecayPolicy{Kind: "none"}}
	s.RegisterFrame(f)

	s.Assert(&Record{ID: "chalmers-1995", Frame: "philosophical", Content: "Chalmers introduced the hard problem in 1995.", Timestamp: now.Add(-31 * 365 * 24 * time.Hour)})
	s.Believe(&Belief{
		ID: "hard-problem-real", Frame: "philosophical",
		Content:    "The hard problem of consciousness is real.",
		Confidence: 0.72, AssertedAt: now.Add(-5 * 24 * time.Hour),
		Derivation: []string{"chalmers-1995"},
	})

	explanation, err := s.Explain("hard-problem-real", now)
	if err != nil {
		t.Fatalf("Explain failed: %v", err)
	}

	t.Logf("\n%s", explanation)

	if !strings.Contains(explanation, "timeless") {
		t.Error("explanation should note philosophical frame is timeless")
	}
}

func TestExplainDownstream(t *testing.T) {
	s, now := setupContractionStore(t) // reuse — b4 depends on b1 depends on r1
	explanation, err := s.Explain("b1", now)
	if err != nil {
		t.Fatalf("Explain failed: %v", err)
	}
	t.Logf("\n%s", explanation)
	if !strings.Contains(explanation, "Downstream") {
		t.Error("explanation should mention downstream impact")
	}
}
