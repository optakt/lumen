package lumen

import (
	"sort"
	"testing"
	"time"
)

func setupQueryStore(t *testing.T) (*Store, time.Time) {
	t.Helper()
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "philosophical", Decay: DecayPolicy{Kind: DecayNone}})
	s.RegisterFrame(Frame{Name: "empirical",     Decay: DecayPolicy{Kind: DecayNone}})
	s.RegisterFrame(Frame{Name: "reasoning",     Decay: DecayPolicy{Kind: DecayNone}})

	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "Cogitate study", Timestamp: now})
	s.Assert(&Record{ID: "r2", Frame: "philosophical", Content: "Chalmers 1995", Timestamp: now})

	s.Believe(&Belief{ID: "hard-problem", Frame: "philosophical", Content: "The hard problem of consciousness is real", Confidence: 0.72, AssertedAt: now, Derivation: []string{"r2"}})
	s.Believe(&Belief{ID: "iit-weakened", Frame: "empirical", Content: "IIT is significantly weakened by Cogitate results", Confidence: 0.70, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "gwt-viable",   Frame: "empirical", Content: "GWT remains viable despite Cogitate", Confidence: 0.65, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "penrose-low",  Frame: "philosophical", Content: "Penrose Orch OR hypothesis involves quantum effects", Confidence: 0.35, AssertedAt: now})

	// Make one suspect
	s.Retract("r1", "test", now)

	return s, now
}

func ids(matches []QueryMatch) []string {
	var result []string
	for _, m := range matches {
		result = append(result, m.BeliefID)
	}
	sort.Strings(result)
	return result
}

func TestQueryConfidence(t *testing.T) {
	s, now := setupQueryStore(t)
	matches, err := s.QueryBeliefs("confidence > 0.7", now)
	if err != nil { t.Fatalf("query error: %v", err) }
	got := ids(matches)
	t.Logf("confidence > 0.7: %v", got)
	for _, m := range matches {
		if m.CurrentConfidence <= 0.7 {
			t.Errorf("belief %s has confidence %.2f, should be > 0.7", m.BeliefID, m.CurrentConfidence)
		}
	}
}

func TestQueryFrame(t *testing.T) {
	s, now := setupQueryStore(t)
	matches, err := s.QueryBeliefs("frame = philosophical", now)
	if err != nil { t.Fatalf("query error: %v", err) }
	got := ids(matches)
	t.Logf("frame = philosophical: %v", got)
	if len(got) != 2 {
		t.Errorf("expected 2 philosophical beliefs, got %d: %v", len(got), got)
	}
}

func TestQueryContentContains(t *testing.T) {
	s, now := setupQueryStore(t)
	matches, err := s.QueryBeliefs(`content contains "consciousness"`, now)
	if err != nil { t.Fatalf("query error: %v", err) }
	got := ids(matches)
	t.Logf("content contains 'consciousness': %v", got)
	if len(got) == 0 {
		t.Error("expected at least one belief containing 'consciousness'")
	}
}

func TestQueryAND(t *testing.T) {
	s, now := setupQueryStore(t)
	matches, err := s.QueryBeliefs("frame = philosophical AND confidence > 0.5", now)
	if err != nil { t.Fatalf("query error: %v", err) }
	got := ids(matches)
	t.Logf("frame=philosophical AND confidence>0.5: %v", got)
	// hard-problem (0.72, philosophical) should match; penrose-low (0.35) should not
	for _, id := range got {
		if id == "penrose-low" {
			t.Error("penrose-low should be excluded (confidence=0.35)")
		}
	}
}

func TestQueryOR(t *testing.T) {
	s, now := setupQueryStore(t)
	matches, err := s.QueryBeliefs(`id = "hard-problem" OR id = "gwt-viable"`, now)
	if err != nil { t.Fatalf("query error: %v", err) }
	got := ids(matches)
	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d: %v", len(got), got)
	}
}

func TestQueryNOT(t *testing.T) {
	s, now := setupQueryStore(t)
	matches, err := s.QueryBeliefs("NOT state = suspect", now)
	if err != nil { t.Fatalf("query error: %v", err) }
	got := ids(matches)
	t.Logf("NOT state=suspect: %v", got)
	for _, m := range matches {
		if m.State == BeliefSuspect {
			t.Errorf("belief %s is suspect but should be excluded", m.BeliefID)
		}
	}
}

func TestQueryGrouping(t *testing.T) {
	s, now := setupQueryStore(t)
	matches, err := s.QueryBeliefs("(frame = empirical OR frame = philosophical) AND confidence >= 0.65", now)
	if err != nil { t.Fatalf("query error: %v", err) }
	got := ids(matches)
	t.Logf("(frame=empirical OR frame=philosophical) AND confidence>=0.65: %v", got)
	for _, m := range matches {
		if m.CurrentConfidence < 0.65 {
			t.Errorf("belief %s has confidence %.2f below threshold", m.BeliefID, m.CurrentConfidence)
		}
	}
}

func TestQueryParseError(t *testing.T) {
	s, now := setupQueryStore(t)
	_, err := s.QueryBeliefs("confidence > > 0.5", now)
	if err == nil {
		t.Error("expected parse error for malformed query")
	}
	t.Logf("parse error (expected): %v", err)
}

func TestQueryTypeErrors(t *testing.T) {
	s, now := setupQueryStore(t)
	cases := []struct {
		query string
		desc  string
	}{
		{`confidence = "high"`, "string value for numeric field"},
		{`frame > 0.5`, "numeric value for string field"},
		{`unknown = "x"`, "unknown field"},
	}
	for _, tc := range cases {
		_, err := s.QueryBeliefs(tc.query, now)
		if err == nil {
			t.Errorf("expected parse error for %s (%q), got none", tc.desc, tc.query)
		} else {
			t.Logf("correctly rejected %s: %v", tc.desc, err)
		}
	}
}
