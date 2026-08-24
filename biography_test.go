package lumen

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestEpistemicBiographyBasic(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t4"].Add(time.Hour)

	bio, err := s.EpistemicBiography("b1", 0.05, now)
	if err != nil {
		t.Fatalf("EpistemicBiography: %v", err)
	}

	if bio.BeliefID != "b1" {
		t.Errorf("BeliefID: got %s", bio.BeliefID)
	}
	if bio.Frame != "test" {
		t.Errorf("Frame: got %s", bio.Frame)
	}

	// Confidence arc: initial + 2 changes = 3 points (initial, after t1, after t3)
	if len(bio.ConfidenceArc) < 2 {
		t.Errorf("expected at least 2 points in confidence arc, got %d", len(bio.ConfidenceArc))
	}
	t.Logf("Confidence arc (%d points):", len(bio.ConfidenceArc))
	for _, pt := range bio.ConfidenceArc {
		t.Logf("  %s  %.0f%%  [%s]", pt.At.Format("2006-01-02"), pt.Confidence*100, pt.Label)
	}
}

func TestEpistemicBiographyMindChanges(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t4"].Add(time.Hour)

	// Threshold 0.05: both changes (15pp and -20pp) should qualify.
	bio, err := s.EpistemicBiography("b1", 0.05, now)
	if err != nil {
		t.Fatalf("EpistemicBiography: %v", err)
	}

	if len(bio.MindChanges) != 2 {
		t.Fatalf("expected 2 mind changes, got %d", len(bio.MindChanges))
	}

	// First: strengthened (60→75, +15pp, moderate)
	mc0 := bio.MindChanges[0]
	if mc0.Direction != "strengthened" {
		t.Errorf("mind change 0: expected strengthened, got %s", mc0.Direction)
	}
	if mc0.Magnitude != "large" {
		t.Errorf("mind change 0: expected large, got %s", mc0.Magnitude)
	}
	t.Logf("mind change 0: %s %.0fpp [%s] %s", mc0.Direction, mc0.Delta*100, mc0.Magnitude, mc0.Reason)

	// Second: weakened (75→55, -20pp, large)
	mc1 := bio.MindChanges[1]
	if mc1.Direction != "weakened" {
		t.Errorf("mind change 1: expected weakened, got %s", mc1.Direction)
	}
	if mc1.Magnitude != "large" {
		t.Errorf("mind change 1: expected large, got %s", mc1.Magnitude)
	}
	t.Logf("mind change 1: %s %.0fpp [%s]", mc1.Direction, mc1.Delta*100, mc1.Magnitude)
}

func TestEpistemicBiographyThreshold(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t4"].Add(time.Hour)

	// Threshold 0.18: only the -20pp drop qualifies (0.20 >= 0.18 but 0.15 < 0.18).
	bio, err := s.EpistemicBiography("b1", 0.18, now)
	if err != nil {
		t.Fatalf("EpistemicBiography: %v", err)
	}

	if len(bio.MindChanges) != 1 {
		t.Fatalf("expected 1 mind change above threshold 0.25, got %d", len(bio.MindChanges))
	}
	if bio.MindChanges[0].Direction != "weakened" {
		t.Errorf("expected weakened, got %s", bio.MindChanges[0].Direction)
	}
	t.Logf("above 25pp threshold: %s %.0fpp", bio.MindChanges[0].Direction, bio.MindChanges[0].Delta*100)
}

func TestEpistemicBiographyRetractions(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t4"].Add(time.Hour)

	bio, err := s.EpistemicBiography("b1", 0.0, now)
	if err != nil {
		t.Fatalf("EpistemicBiography: %v", err)
	}

	// r2 was retracted with "data corruption" and is in current chain.
	found := false
	for _, ev := range bio.Retractions {
		if ev.RecordID == "r2" && ev.RetractReason == "data corruption" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected r2 retraction in biography, retractions: %v", bio.Retractions)
	}
	t.Logf("retractions: %d", len(bio.Retractions))
}

func TestEpistemicBiographyNoRevisions(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{Name: "test", Decay: DecayPolicy{Kind: "none"}})
	now := time.Now()
	_ = s.Assert(&Record{ID: "r1", Content: "R1", Timestamp: now, Frame: "test"})
	_ = s.Believe(&Belief{
		ID: "b-stable", Content: "Never revised.", Confidence: 0.80,
		AssertedAt: now, Frame: "test", Derivation: []string{"r1"},
	})

	bio, err := s.EpistemicBiography("b-stable", 0.05, now)
	if err != nil {
		t.Fatalf("EpistemicBiography: %v", err)
	}

	if len(bio.MindChanges) != 0 {
		t.Errorf("expected 0 mind changes for unrevised belief, got %d", len(bio.MindChanges))
	}
	if len(bio.ConfidenceArc) != 1 {
		t.Errorf("expected 1 arc point for unrevised belief, got %d", len(bio.ConfidenceArc))
	}
	t.Logf("stable belief: arc=%d changes=%d", len(bio.ConfidenceArc), len(bio.MindChanges))
}

func TestEpistemicBiographyNotFound(t *testing.T) {
	s := NewStore()
	_, err := s.EpistemicBiography("ghost", 0.05, time.Now())
	if err == nil {
		t.Error("expected error for missing belief, got nil")
	}
}

func TestClassifyMagnitude(t *testing.T) {
	cases := []struct {
		delta float64
		want  string
	}{
		{0.35, "decisive"},
		{0.30, "decisive"},
		{0.20, "large"},
		{0.15, "large"},
		{0.10, "moderate"},
		{0.07, "moderate"},
		{0.05, "small"},
		{0.01, "small"},
	}
	for _, tc := range cases {
		got := classifyMagnitude(tc.delta)
		if got != tc.want {
			t.Errorf("classifyMagnitude(%.2f) = %s, want %s", tc.delta, got, tc.want)
		}
	}
}

func TestRenderBiography(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t4"].Add(time.Hour)

	bio, err := s.EpistemicBiography("b1", 0.05, now)
	if err != nil {
		t.Fatalf("EpistemicBiography: %v", err)
	}

	rendered := RenderBiography(bio)
	if rendered == "" {
		t.Fatal("RenderBiography returned empty string")
	}

	// Check key sections are present.
	for _, section := range []string{
		"Epistemic Biography",
		"Confidence trajectory",
		"Mind changes",
		"Retractions",
	} {
		if !strings.Contains(rendered, section) {
			t.Errorf("rendered biography missing section: %q", section)
		}
	}
	t.Log("\n" + rendered)
}

func TestDecayTrajectory(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(Frame{
		Name:  "decaying",
		Decay: DecayPolicy{Kind: "exponential", Halflife: 365 * 24 * time.Hour},
	})
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = s.Assert(&Record{ID: "r1", Frame: "decaying", Content: "A.", Timestamp: t0})
	_ = s.Believe(&Belief{ID: "b1", Frame: "decaying", Content: "Decaying belief.", Confidence: 0.80, AssertedAt: t0, Derivation: []string{"r1"}})

	// Query 3 years later — should show significant decay.
	t3y := t0.Add(3 * 365 * 24 * time.Hour)
	bio, err := s.EpistemicBiography("b1", 0.05, t3y)
	if err != nil {
		t.Fatalf("bio: %v", err)
	}

	if len(bio.DecayTrajectory) == 0 {
		t.Fatal("decaying frame should populate DecayTrajectory")
	}
	first := bio.DecayTrajectory[0]
	last  := bio.DecayTrajectory[len(bio.DecayTrajectory)-1]
	t.Logf("Decay trajectory: %.0f%% → %.0f%% over 3 years", first.Confidence*100, last.Confidence*100)

	if last.Confidence >= first.Confidence {
		t.Errorf("confidence should have decayed: %.3f >= %.3f", last.Confidence, first.Confidence)
	}

	// After 3 halflives (3 years with 1y halflife) confidence should be ~10% of original.
	expected := 0.80 * 0.125 // 2^(-3) = 0.125
	if math.Abs(last.Confidence-expected) > 0.02 {
		t.Errorf("after 3 halflives: expected ~%.3f, got %.3f", expected, last.Confidence)
	}
}
