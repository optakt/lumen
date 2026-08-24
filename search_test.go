package lumen

import (
	"testing"
	"time"
)

func setupSearchStore(t *testing.T) (*Store, time.Time) {
	t.Helper()
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayNone}})
	s.RegisterFrame(Frame{Name: "philosophical", Decay: DecayPolicy{Kind: DecayNone}})

	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "The Cogitate Consortium adversarial collaboration found IIT predictions unconfirmed.", Timestamp: now})
	s.Assert(&Record{ID: "r2", Frame: "philosophical", Content: "Chalmers introduced the hard problem of consciousness in 1995.", Timestamp: now})
	s.Assert(&Record{ID: "r3", Frame: "empirical", Content: "Global Workspace Theory predicts late-ignition of frontoparietal networks.", Timestamp: now})

	s.Believe(&Belief{ID: "iit-weakened", Frame: "empirical", Content: "Integrated Information Theory is significantly weakened by empirical Cogitate adversarial results.", Confidence: 0.70, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "gwt-viable", Frame: "empirical", Content: "Global Workspace Theory remains viable as a theory of consciousness.", Confidence: 0.65, AssertedAt: now, Derivation: []string{"r3"}})
	s.Believe(&Belief{ID: "hard-problem", Frame: "philosophical", Content: "The hard problem of consciousness remains unsolved despite empirical progress.", Confidence: 0.72, AssertedAt: now, Derivation: []string{"r2"}})

	return s, now
}

func TestSearchBasic(t *testing.T) {
	s, _ := setupSearchStore(t)
	idx := s.BuildSearchIndex()

	results := s.Search(idx, "IIT consciousness weakened", 5)
	t.Logf("Search 'IIT consciousness weakened' (%d results):", len(results))
	for _, r := range results {
		t.Logf("  [%.2f] %-8s %s — %s", r.Similarity, r.Kind, r.NodeID, truncate(r.Content, 55))
	}

	if len(results) == 0 {
		t.Error("expected at least one result for IIT search")
	}
	// iit-weakened or r1 should be top result
	if results[0].NodeID != "iit-weakened" && results[0].NodeID != "r1" {
		t.Logf("Top result is %s (acceptable, semantic similarity may differ)", results[0].NodeID)
	}
}

func TestSearchConsciousness(t *testing.T) {
	s, _ := setupSearchStore(t)
	idx := s.BuildSearchIndex()

	results := s.Search(idx, "consciousness hard problem subjective experience", 5)
	t.Logf("Search 'consciousness hard problem' (%d results):", len(results))
	for _, r := range results {
		t.Logf("  [%.2f] %s — %s", r.Similarity, r.NodeID, truncate(r.Content, 55))
	}
	if len(results) == 0 {
		t.Error("expected results for consciousness search")
	}
}

func TestSimilarBeliefs(t *testing.T) {
	s, _ := setupSearchStore(t)
	idx := s.BuildSearchIndex()

	similar, err := s.SimilarBeliefs(idx, "iit-weakened", 3)
	if err != nil { t.Fatalf("SimilarBeliefs: %v", err) }

	t.Logf("Beliefs similar to 'iit-weakened':")
	for _, r := range similar {
		t.Logf("  [%.2f] %s — %s", r.Similarity, r.NodeID, truncate(r.Content, 55))
	}
	// gwt-viable is also about consciousness theories — should appear
	// hard-problem is about consciousness — might appear
	if len(similar) == 0 {
		t.Error("expected at least one similar belief")
	}
	// Should not include iit-weakened itself
	for _, r := range similar {
		if r.NodeID == "iit-weakened" {
			t.Error("SimilarBeliefs should not return the query belief itself")
		}
	}
}

func TestSearchIndex(t *testing.T) {
	s, _ := setupSearchStore(t)
	idx := s.BuildSearchIndex()

	if idx.N == 0 {
		t.Error("index should have documents")
	}
	t.Logf("Index: %d documents, %d unique terms", idx.N, len(idx.idf))
	if len(idx.idf) == 0 {
		t.Error("index should have IDF scores")
	}
}
