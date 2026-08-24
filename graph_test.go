package lumen

import (
	"testing"
)

func TestBeliefGraphBasic(t *testing.T) {
	g := NewBeliefGraph()

	// A derives from B
	g.AddEdge(Edge{From: "B", To: "A-derived", Kind: EdgeDerives, Label: "A infers from B"})
	// A-response references B but does not derive
	g.AddEdge(Edge{From: "A-response", To: "B", Kind: EdgeReferences, Label: "A responds to B"})
	// A-contrast contrasts with A-derived
	g.AddEdge(Edge{From: "A-contrast", To: "A-derived", Kind: EdgeContrasts})

	// Derivation cascade: what can we reach from B via derivation?
	reachable := g.ReachableByDerivation("B")
	if len(reachable) != 1 || reachable[0] != "A-derived" {
		t.Errorf("expected [A-derived], got %v", reachable)
	}

	// A-response is NOT reachable by derivation from B
	for _, id := range reachable {
		if id == "A-response" {
			t.Error("A-response should not be reachable via derivation from B")
		}
	}

	// Semantic neighbors of B include A-response (references B)
	neighbors := g.SemanticNeighbors("B")
	found := false
	for _, n := range neighbors {
		if n == "A-response" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected A-response in semantic neighbors of B, got %v", neighbors)
	}
}

func TestBeliefGraphNoDuplicateEdges(t *testing.T) {
	g := NewBeliefGraph()
	g.AddEdge(Edge{From: "X", To: "Y", Kind: EdgeDerives})
	g.AddEdge(Edge{From: "X", To: "Y", Kind: EdgeDerives}) // duplicate
	g.AddEdge(Edge{From: "X", To: "Y", Kind: EdgeReferences}) // different kind, should be added

	stats := g.Stats()
	if stats[EdgeDerives] != 1 {
		t.Errorf("expected 1 EdgeDerives, got %d", stats[EdgeDerives])
	}
	if stats[EdgeReferences] != 1 {
		t.Errorf("expected 1 EdgeReferences, got %d", stats[EdgeReferences])
	}
}

func TestBeliefGraphValidate(t *testing.T) {
	g := NewBeliefGraph()
	g.AddEdge(Edge{From: "A", To: "B", Kind: EdgeDerives})
	if err := g.Validate(); err != nil {
		t.Errorf("expected valid graph, got: %v", err)
	}
}

func TestBeliefGraphSelfLoop(t *testing.T) {
	g := NewBeliefGraph()
	// Manually bypass duplicate check by using different kinds
	g.edges = append(g.edges, Edge{From: "A", To: "A", Kind: EdgeDerives})
	g.outbound["A"] = append(g.outbound["A"], 0)
	g.inbound["A"] = append(g.inbound["A"], 0)
	if err := g.Validate(); err == nil {
		t.Error("expected self-loop error")
	}
}

func TestBeliefGraphTransitiveDerivation(t *testing.T) {
	g := NewBeliefGraph()
	// A → B → C → D (derivation chain)
	g.AddEdge(Edge{From: "A", To: "B", Kind: EdgeDerives})
	g.AddEdge(Edge{From: "B", To: "C", Kind: EdgeDerives})
	g.AddEdge(Edge{From: "C", To: "D", Kind: EdgeDerives})
	// E references C but does not derive
	g.AddEdge(Edge{From: "E", To: "C", Kind: EdgeReferences})

	reachable := g.ReachableByDerivation("A")
	if len(reachable) != 3 {
		t.Errorf("expected 3 reachable via derivation, got %d: %v", len(reachable), reachable)
	}
	// E should not be reachable
	for _, id := range reachable {
		if id == "E" {
			t.Error("E should not be reachable via derivation chain")
		}
	}
}

func TestBeliefGraphDebatePattern(t *testing.T) {
	// Reproduce the debate simulator situation:
	// hp-circuit-reframe is a *response* to il-introspection-unreliable
	// but must NOT be retracted when the latter is retracted.
	g := NewBeliefGraph()

	// il-introspection-unreliable derives from il-consciousness-data (illusionist evidence)
	g.AddEdge(Edge{
		From: "il-consciousness-data",
		To:   "il-introspection-unreliable",
		Kind: EdgeDerives,
		Label: "evidence for introspection unreliability",
	})
	// hp-circuit-reframe REFERENCES il-introspection-unreliable (responds to it)
	// but does NOT derive — it takes the evidence and reframes it differently
	g.AddEdge(Edge{
		From:  "hp-circuit-reframe",
		To:    "il-introspection-unreliable",
		Kind:  EdgeReferences,
		Label: "HP theorist's reframing of circuit-tracing evidence",
	})

	// Retraction cascade from il-consciousness-data should only reach il-introspection-unreliable
	reachable := g.ReachableByDerivation("il-consciousness-data")
	for _, id := range reachable {
		if id == "hp-circuit-reframe" {
			t.Error("hp-circuit-reframe must not be retracted when illusionist evidence is retracted")
		}
	}

	// But hp-circuit-reframe IS a semantic neighbor of il-introspection-unreliable
	neighbors := g.SemanticNeighbors("il-introspection-unreliable")
	found := false
	for _, n := range neighbors {
		if n == "hp-circuit-reframe" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected hp-circuit-reframe as semantic neighbor of il-introspection-unreliable, got %v", neighbors)
	}
}
