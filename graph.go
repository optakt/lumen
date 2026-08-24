package lumen

import (
	"fmt"
	"sync"
)

// EdgeKind classifies the relationship between two nodes in the belief graph.
type EdgeKind string

const (
	// EdgeDerives means A was inferred from B. Retraction of B suspects A.
	EdgeDerives EdgeKind = "derives"
	// EdgeReferences means A is about, responds to, or contrasts with B.
	// No retraction propagation — the relationship is epistemic, not inferential.
	EdgeReferences EdgeKind = "references"
	// EdgeContrasts means A explicitly opposes or challenges B.
	// Also no retraction propagation. Used in debate/dialectic structures.
	EdgeContrasts EdgeKind = "contrasts"
	// EdgeExtends means A elaborates or specializes B without full derivation.
	EdgeExtends EdgeKind = "extends"
)

// Edge is a directed typed relationship between two nodes (beliefs or records).
type Edge struct {
	From string
	To   string
	Kind EdgeKind
	// Optional human-readable label for why this edge exists.
	Label string
}

// BeliefGraph holds the full relationship structure across all belief nodes.
// It separates the derivation graph (which drives retraction) from semantic
// relationships (which are epistemic but non-inferential).
type BeliefGraph struct {
	mu    sync.RWMutex
	edges []Edge
	// outbound[from] → list of edge indices (into edges slice)
	outbound map[string][]int
	// inbound[to] → list of edge indices
	inbound map[string][]int
}

func NewBeliefGraph() *BeliefGraph {
	return &BeliefGraph{
		outbound: make(map[string][]int),
		inbound:  make(map[string][]int),
	}
}

// AddEdge records a directed typed edge. Duplicate edges (same from/to/kind) are silently ignored.
func (g *BeliefGraph) AddEdge(e Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// Check for duplicate
	for _, i := range g.outbound[e.From] {
		existing := g.edges[i]
		if existing.To == e.To && existing.Kind == e.Kind {
			return
		}
	}
	idx := len(g.edges)
	g.edges = append(g.edges, e)
	g.outbound[e.From] = append(g.outbound[e.From], idx)
	g.inbound[e.To] = append(g.inbound[e.To], idx)
}

// EdgesFrom returns all edges outbound from the given node.
func (g *BeliefGraph) EdgesFrom(id string) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	idxs := g.outbound[id]
	result := make([]Edge, len(idxs))
	for i, idx := range idxs {
		result[i] = g.edges[idx]
	}
	return result
}

// EdgesTo returns all edges inbound to the given node.
func (g *BeliefGraph) EdgesTo(id string) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	idxs := g.inbound[id]
	result := make([]Edge, len(idxs))
	for i, idx := range idxs {
		result[i] = g.edges[idx]
	}
	return result
}

// DerivationSources returns the IDs of all nodes A derives from.
func (g *BeliefGraph) DerivationSources(id string) []string {
		return g.nodesOfKind(id, EdgeDerives, true)
}

// SemanticNeighbors returns all nodes that share a semantic (non-inferential) edge with id.
// Direction is ignored — both from and to are included.
func (g *BeliefGraph) SemanticNeighbors(id string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	seen := make(map[string]bool)
	var result []string
	add := func(other string) {
		if !seen[other] && other != id {
			seen[other] = true
			result = append(result, other)
		}
	}
	semanticKinds := map[EdgeKind]bool{
		EdgeReferences: true,
		EdgeContrasts:  true,
		EdgeExtends:    true,
	}
	for _, idx := range g.outbound[id] {
		e := g.edges[idx]
		if semanticKinds[e.Kind] {
			add(e.To)
		}
	}
	for _, idx := range g.inbound[id] {
		e := g.edges[idx]
		if semanticKinds[e.Kind] {
			add(e.From)
		}
	}
	return result
}

// ReachableByDerivation returns all node IDs reachable from `id` via EdgeDerives edges.
// Used for retraction cascade: which beliefs derive (transitively) from this node?
func (g *BeliefGraph) ReachableByDerivation(id string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	visited := make(map[string]bool)
	var result []string
	var visit func(string)
	visit = func(cur string) {
		for _, idx := range g.outbound[cur] {
			e := g.edges[idx]
			if e.Kind == EdgeDerives && !visited[e.To] {
				visited[e.To] = true
				result = append(result, e.To)
				visit(e.To)
			}
		}
	}
	visit(id)
	return result
}

// Validate checks graph invariants: no self-loops, no unknown edge kinds.
func (g *BeliefGraph) Validate() error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	valid := map[EdgeKind]bool{
		EdgeDerives: true, EdgeReferences: true,
		EdgeContrasts: true, EdgeExtends: true,
	}
	for _, e := range g.edges {
		if e.From == e.To {
			return fmt.Errorf("self-loop on node %s", e.From)
		}
		if !valid[e.Kind] {
			return fmt.Errorf("unknown edge kind %q", e.Kind)
		}
	}
	return nil
}

// Stats returns a summary of graph structure.
func (g *BeliefGraph) Stats() map[EdgeKind]int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	counts := make(map[EdgeKind]int)
	for _, e := range g.edges {
		counts[e.Kind]++
	}
	return counts
}

// nodesOfKind returns IDs connected to `id` via edges of the given kind.
// If outbound is true, returns the To nodes of outbound edges from id.
// If false, returns the From nodes of inbound edges to id.
func (g *BeliefGraph) nodesOfKind(id string, kind EdgeKind, outbound bool) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var result []string
	if outbound {
		for _, idx := range g.outbound[id] {
			e := g.edges[idx]
			if e.Kind == kind {
				result = append(result, e.To)
			}
		}
	} else {
		for _, idx := range g.inbound[id] {
			e := g.edges[idx]
			if e.Kind == kind {
				result = append(result, e.From)
			}
		}
	}
	return result
}

// RemoveNode removes a node and all its edges from the graph.
// Used by ApplyContraction and Recover to manage graph integrity.
// Rebuilds the outbound/inbound index maps after removal to prevent
// stale indices from causing panics when edges are later added.
func (g *BeliefGraph) RemoveNode(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// Filter out all edges involving this node.
	var filtered []Edge
	for _, e := range g.edges {
		if e.From != id && e.To != id {
			filtered = append(filtered, e)
		}
	}
	g.edges = filtered
	// Rebuild index maps from scratch — stale indices from before the
	// removal would point to wrong positions in the shrunken slice.
	g.outbound = make(map[string][]int, len(g.outbound))
	g.inbound = make(map[string][]int, len(g.inbound))
	for i, e := range g.edges {
		g.outbound[e.From] = append(g.outbound[e.From], i)
		g.inbound[e.To] = append(g.inbound[e.To], i)
	}
}
