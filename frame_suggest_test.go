package lumen

import (
	"fmt"
	"testing"
	"time"
)

func TestSuggestFramePhilosophical(t *testing.T) {
	texts := []string{
		"The hard problem of consciousness is the challenge of explaining why physical processes give rise to qualia.",
		"Chalmers argues for property dualism: consciousness is a fundamental ontological category.",
	}
	for _, text := range texts {
		s := SuggestFrame(text)
		if s.Frame != "philosophical" {
			t.Errorf("expected philosophical for %q..., got %s (reasoning: %s)", text[:50], s.Frame, s.Reasoning)
		}
		t.Logf("philosophical: conf=%.2f  %s", s.Confidence, s.Reasoning)
	}
}

func TestSuggestFrameEmpirical(t *testing.T) {
	texts := []string{
		"The experiment measured n=256 subjects and found a statistically significant correlation (p < 0.05).",
		"Brain scan data from fMRI studies shows activation in prefrontal cortex during the task.",
	}
	for _, text := range texts {
		s := SuggestFrame(text)
		if s.Frame != "empirical" {
			t.Errorf("expected empirical for %q..., got %s", text[:50], s.Frame)
		}
		t.Logf("empirical: conf=%.2f  %s", s.Confidence, s.Reasoning)
	}
}

func TestSuggestFrameReasoning(t *testing.T) {
	text := "Therefore, if the premise holds, the conclusion follows by deductive entailment."
	s := SuggestFrame(text)
	if s.Frame != "reasoning" {
		t.Errorf("expected reasoning, got %s", s.Frame)
	}
	t.Logf("reasoning: conf=%.2f  %s", s.Confidence, s.Reasoning)
}

func TestSuggestFrameContemporary(t *testing.T) {
	text := "The current research consensus holds that the prevailing view in the field has shifted."
	s := SuggestFrame(text)
	if s.Frame != "contemporary" {
		t.Errorf("expected contemporary, got %s (reasoning: %s)", s.Frame, s.Reasoning)
	}
	t.Logf("contemporary: conf=%.2f  %s", s.Confidence, s.Reasoning)
}

func TestSuggestFrameForBelief(t *testing.T) {
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "empirical",     Decay: DecayPolicy{Kind: "none"}})
	s.RegisterFrame(Frame{Name: "philosophical", Decay: DecayPolicy{Kind: "none"}})

	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "Study found significant correlation", Timestamp: now})
	s.Believe(&Belief{
		ID: "mismatch", Frame: "empirical",
		Content:    "The hard problem of consciousness with qualia remains unsolved despite empirical findings.",
		Confidence: 0.7, AssertedAt: now, Derivation: []string{"r1"},
	})

	suggestion := s.SuggestFrameForBelief("mismatch")
	t.Logf("Frame suggestion for 'mismatch': %s (conf=%.2f)  %s",
		suggestion.Frame, suggestion.Confidence, suggestion.Reasoning)
	if suggestion.Frame != "philosophical" {
		t.Errorf("expected philosophical suggestion for consciousness/qualia content, got %s", suggestion.Frame)
	}
}

func formatScores(m map[string]float64) string {
	var parts []string
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s=%.2f", k, v))
	}
	return fmt.Sprint(parts)
}
