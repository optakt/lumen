package lumen

import (
	"strings"
	"testing"
	"time"
)

func setupProvenanceStore(t *testing.T) (*Store, time.Time) {
	t.Helper()
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "empirical",     Decay: DecayPolicy{Kind: DecayNone}})
	s.RegisterFrame(Frame{Name: "philosophical", Decay: DecayPolicy{Kind: DecayNone}})

	// Chain: r1 → b1 → b3
	//        r2 → b2 → b3
	//        r3 (independent) → b4
	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "Cogitate 2023 finding", Timestamp: now.AddDate(-1, 0, 0)})
	s.Assert(&Record{ID: "r2", Frame: "empirical", Content: "Independent replication", Timestamp: now.AddDate(-1, 0, 0)})
	s.Assert(&Record{ID: "r3", Frame: "philosophical", Content: "Chalmers 1995 argument", Timestamp: now.AddDate(-2, 0, 0)})
	s.Believe(&Belief{ID: "b1", Frame: "empirical", Content: "IIT weakened by Cogitate", Confidence: 0.70, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "b2", Frame: "empirical", Content: "IIT weakened by replication failure", Confidence: 0.65, AssertedAt: now, Derivation: []string{"r2"}})
	s.Believe(&Belief{ID: "b3", Frame: "empirical", Content: "IIT is substantially weakened", Confidence: 0.88, AssertedAt: now, Derivation: []string{"b1", "b2"}})
	s.Believe(&Belief{ID: "b4", Frame: "philosophical", Content: "Hard problem remains unsolved", Confidence: 0.72, AssertedAt: now, Derivation: []string{"r3"}})

	return s, now
}

func TestProvenanceChainBasic(t *testing.T) {
	s, now := setupProvenanceStore(t)

	chain, err := s.ProvenanceChain("b3", now)
	if err != nil { t.Fatalf("ProvenanceChain: %v", err) }

	// Should include b3, b1, b2, r1, r2 — but NOT b4 or r3
	for _, id := range []string{"b3", "b1", "b2", "r1", "r2"} {
		if _, ok := chain.Nodes[id]; !ok {
			t.Errorf("expected %s in provenance chain, not found", id)
		}
	}
	for _, id := range []string{"b4", "r3"} {
		if _, ok := chain.Nodes[id]; ok {
			t.Errorf("unexpected %s in provenance chain (unrelated)", id)
		}
	}

	t.Logf("Chain stats: depth=%d records=%d nodes=%d", chain.MaxDepth, chain.TotalRecords, len(chain.Nodes))
	t.Logf("\n%s", chain.Render())
}

func TestProvenanceDepths(t *testing.T) {
	s, now := setupProvenanceStore(t)
	chain, _ := s.ProvenanceChain("b3", now)

	// b3 is depth 0, b1/b2 are depth 1, r1/r2 are depth 2
	depths := map[string]int{"b3": 0, "b1": 1, "b2": 1, "r1": 2, "r2": 2}
	for id, expectedDepth := range depths {
		node := chain.Nodes[id]
		if node == nil { t.Fatalf("missing node %s", id); continue }
		if node.Depth != expectedDepth {
			t.Errorf("node %s: expected depth %d, got %d", id, expectedDepth, node.Depth)
		}
	}
}

func TestProvenanceWeakestLink(t *testing.T) {
	s, now := setupProvenanceStore(t)
	chain, _ := s.ProvenanceChain("b3", now)

	// b2 (0.65) should be the weakest link
	weak := chain.WeakestLink()
	if weak == nil { t.Fatal("WeakestLink returned nil") }
	t.Logf("Weakest link: %s (%.2f) — %q", weak.ID, weak.Confidence, weak.Content)
	if weak.ID != "b2" {
		t.Errorf("expected b2 as weakest link (0.65), got %s (%.2f)", weak.ID, weak.Confidence)
	}
}

func TestProvenanceConfidencePaths(t *testing.T) {
	s, now := setupProvenanceStore(t)
	chain, _ := s.ProvenanceChain("b3", now)

	paths := chain.ConfidencePaths()
	t.Logf("Confidence paths: %d", len(paths))
	for i, p := range paths {
		var ids []string
		for _, step := range p.Steps { ids = append(ids, step.ID) }
		t.Logf("  Path %d: %s → min=%.2f", i+1, strings.Join(ids, " → "), p.MinConfidence())
	}
	// Should have 2 paths: b3→b1→r1 and b3→b2→r2
	if len(paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(paths))
	}
}

func TestProvenanceWithRetraction(t *testing.T) {
	s, now := setupProvenanceStore(t)
	s.Retract("r1", "study retracted", now)

	chain, _ := s.ProvenanceChain("b3", now)
	if !chain.HasRetracted {
		t.Error("chain should report HasRetracted when source record is retracted")
	}
	r1node := chain.Nodes["r1"]
	if r1node == nil || !r1node.Retracted {
		t.Error("r1 node should show as retracted")
	}
	rendered := chain.Render()
	if !strings.Contains(rendered, "RETRACTED") {
		t.Error("rendered chain should flag retracted node")
	}
	t.Logf("Retracted chain WeakestLink: %s (%.2f)", chain.WeakestLink().ID, chain.WeakestLink().Confidence)
}

func TestFoundationalRecord(t *testing.T) {
	src := `
frame philo
    composition: bayesian
    decay: none

record axiom-of-consciousness in philo
    "Conscious experience exists. This is evident — denying it is self-refuting."
    provenance: foundational
    at: "2026-01-01T00:00:00Z"

record chalmers-1995 in philo
    "Chalmers introduced the hard problem of consciousness in 1995."
    at: "1995-01-01T00:00:00Z"

believe hard-problem in philo
    "The hard problem is real."
    confidence: 0.72
    from: axiom-of-consciousness, chalmers-1995
`
	s := NewStore()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	if err := LoadFile(src, s, now); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// Foundational flag should be set on the record.
	s.mu.RLock()
	axiom, ok := s.records["axiom-of-consciousness"]
	s.mu.RUnlock()
	if !ok {
		t.Fatal("axiom-of-consciousness record not found")
	}
	if !axiom.Foundational {
		t.Error("axiom-of-consciousness should be foundational")
	}
	if chalmers, ok2 := func() (*Record, bool) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		r, ok := s.records["chalmers-1995"]
		return r, ok
	}(); ok2 {
		if chalmers.Foundational {
			t.Error("chalmers-1995 should not be foundational")
		}
	}
	t.Logf("axiom-of-consciousness.Foundational = %v", axiom.Foundational)

	// ProvenanceChain should mark the foundational node.
	chain, err := s.ProvenanceChain("hard-problem", now)
	if err != nil {
		t.Fatalf("ProvenanceChain: %v", err)
	}
	foundNode := chain.Nodes["axiom-of-consciousness"]
	if foundNode == nil {
		t.Fatal("axiom-of-consciousness not in chain")
	}
	if !foundNode.Foundational {
		t.Error("axiom-of-consciousness ProvenanceNode should be Foundational")
	}
	t.Logf("chain rendered:\n%s", chain.Render())

	// WeakestLink should skip the foundational node.
	wl := chain.WeakestLink()
	if wl != nil && wl.ID == "axiom-of-consciousness" {
		t.Error("WeakestLink should not return foundational record")
	}
	t.Logf("weakest link (excluding foundational): %v",
		func() string { if wl == nil { return "none" }; return wl.ID }())

	// Content should NOT be prefixed with "[foundational]" — that was the old hack.
	if len(axiom.Content) > 0 && axiom.Content[:1] == "[" {
		t.Errorf("foundational record content should not start with '[': %s", axiom.Content[:20])
	}
}

func TestFoundationalNotWeakLink(t *testing.T) {
	// When the only records in a chain are foundational, WeakestLink returns nil
	// because foundational nodes are excluded — they are not fragile.
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Decay: DecayPolicy{Kind: DecayNone}})
	now := time.Now()

	must := func(err error) {
		t.Helper()
		if err != nil { t.Fatal(err) }
	}
	must(s.Assert(&Record{
		ID: "axiom1", Content: "First principles.", Frame: "f",
		Timestamp: now, Foundational: true,
	}))
	must(s.Believe(&Belief{
		ID: "b1", Content: "Derived from axiom.", Confidence: 0.80,
		Frame: "f", AssertedAt: now, Derivation: []string{"axiom1"},
	}))

	chain, err := s.ProvenanceChain("b1", now)
	if err != nil {
		t.Fatalf("ProvenanceChain: %v", err)
	}
	wl := chain.WeakestLink()
	if wl != nil {
		t.Errorf("expected nil weakest link (only foundational records), got %s", wl.ID)
	}
	t.Log("weakest link when only foundational records: nil (correct)")
}
