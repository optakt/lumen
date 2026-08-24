package lumen

import (
	"sort"
	"testing"
)

func TestEntityGraphBasic(t *testing.T) {
	g := NewEntityGraph()

	g.RegisterEntity(&Entity{
		ID:      "Gödel",
		Aliases: []string{"Kurt Gödel", "K. Gödel"},
		Kind:    "person",
	})
	g.RegisterEntity(&Entity{
		ID:      "IIT",
		Aliases: []string{"Integrated Information Theory", "Phi theory"},
		Kind:    "theory",
	})

	// Add mentions
	g.AddMention("belief-1", "Kurt Gödel", "Gödel's incompleteness theorems")
	g.AddMention("belief-1", "IIT", "Integrated Information Theory")
	g.AddMention("belief-2", "Gödel", "Gödel's theorem changes everything")
	g.AddMention("belief-3", "IIT", "phi measure in IIT")

	// Nodes for Gödel
	nodes := g.NodesForEntity("gödel")
	sort.Strings(nodes)
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes for Gödel, got %v", nodes)
	}

	// Entities for belief-1
	entities := g.EntitiesForNode("belief-1")
	if len(entities) != 2 {
		t.Errorf("expected 2 entities for belief-1, got %v", entities)
	}

	// Co-mentioned: belief-2 shares Gödel with belief-1
	co := g.CoMentioned("belief-1", 1)
	if len(co) == 0 {
		t.Error("expected co-mentioned nodes")
	}
	found := false
	for _, c := range co {
		if c.NodeID == "belief-2" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected belief-2 in co-mentioned, got %v", co)
	}
}

func TestEntityGraphExtractAndIndex(t *testing.T) {
	g := NewEntityGraph()
	g.RegisterEntity(&Entity{
		ID:      "David Chalmers",
		Aliases: []string{"Chalmers"},
		Kind:    "person",
	})
	g.RegisterEntity(&Entity{
		ID:      "hard problem",
		Aliases: []string{"explanatory gap"},
		Kind:    "concept",
	})

	text := "Chalmers introduced the hard problem of consciousness in 1995, identifying the explanatory gap between physical and phenomenal descriptions."
	found := g.ExtractAndIndex("b1", text)
	if len(found) < 2 {
		t.Errorf("expected at least 2 entities extracted, got %v", found)
	}

	nodes := g.NodesForEntity("david chalmers")
	if len(nodes) != 1 || nodes[0] != "b1" {
		t.Errorf("expected [b1] for Chalmers, got %v", nodes)
	}
}

func TestSimpleNER(t *testing.T) {
	text := "Kurt Gödel proved that any consistent formal system capable of arithmetic contains true statements it cannot prove. Alan Turing showed that the halting problem is undecidable."
	candidates := SimpleNER(text)
	// Should find Kurt Gödel and Alan Turing
	foundGodel, foundTuring := false, false
	for _, c := range candidates {
		if c == "Kurt" || c == "Kurt Gödel" {
			foundGodel = true
		}
		if c == "Alan" || c == "Alan Turing" {
			foundTuring = true
		}
	}
	t.Logf("NER candidates: %v", candidates)
	if !foundGodel {
		t.Error("expected to find Gödel/Kurt")
	}
	if !foundTuring {
		t.Error("expected to find Turing/Alan")
	}
}

func TestEntityGraphNormalization(t *testing.T) {
	g := NewEntityGraph()
	g.RegisterEntity(&Entity{ID: "Immanuel Kant", Aliases: []string{"Kant", "I. Kant"}})

	// All these should resolve to the same entity
	aliases := []string{"kant", "Kant", "I. Kant", "immanuel kant"}
	for _, a := range aliases {
		canon := g.Resolve(a)
		if canon == "" {
			t.Errorf("expected Kant to resolve from %q, got empty", a)
		}
	}
}

func TestEntityGraphDeduplication(t *testing.T) {
	g := NewEntityGraph()
	g.RegisterEntity(&Entity{ID: "Spinoza"})

	// Adding same mention twice should not create duplicate edges
	g.AddMention("b1", "Spinoza", "Spinoza's Ethics")
	g.AddMention("b1", "Spinoza", "Spinoza's Ethics")

	_, mentions := g.EntityStats()
	if mentions != 1 {
		t.Errorf("expected 1 mention, got %d", mentions)
	}
}
