package lumen

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ProvenanceNode represents one node in a belief's complete evidential ancestry.
type ProvenanceNode struct {
	ID          string
	Kind        string // "record" or "belief"
	Content     string
	Frame       string
	Confidence  float64
	AssertedAt  time.Time
	Retracted   bool
	Foundational bool // true for records that are the explicit chain terminus (axioms)
	Depth       int      // 0 = the belief itself, 1 = direct sources, 2 = their sources, etc.
	Children    []string // IDs of nodes this one directly provides evidence for (in this chain)
	Parents     []string // IDs of nodes this one derives from (its own sources)
}

// ProvenanceChain is the complete evidential ancestry of a belief.
type ProvenanceChain struct {
	// Root is the belief whose provenance is being traced.
	Root string
	// Nodes is all nodes in the ancestry, keyed by ID.
	Nodes map[string]*ProvenanceNode
	// DepthOrder is node IDs sorted by depth then ID (for stable printing).
	DepthOrder []string
	// MaxDepth is the deepest depth in the chain.
	MaxDepth int
	// TotalRecords is the number of distinct records (leaf nodes) in the chain.
	TotalRecords int
	// HasRetracted is true if any node in the chain has been retracted.
	HasRetracted bool
}

// ProvenanceChain returns the complete evidential ancestry of a belief,
// walking the derivation graph recursively until it reaches records (leaf nodes).
//
// The result shows the full evidential support for the belief: every record,
// every intermediate belief, their frames, confidences, and retraction status.
// This answers: "what ultimately supports this belief, and how far back does
// the evidence chain go?"
func (s *Store) ProvenanceChain(beliefID string, now time.Time) (*ProvenanceChain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, ok := s.beliefs[beliefID]
	if !ok {
		return nil, fmt.Errorf("belief %s not found", beliefID)
	}
	frame := s.frames[b.Frame]
	conf := b.CurrentConfidence(frame, now)

	chain := &ProvenanceChain{
		Root:  beliefID,
		Nodes: make(map[string]*ProvenanceNode),
	}

	// Add root node
	chain.Nodes[beliefID] = &ProvenanceNode{
		ID:         beliefID,
		Kind:       "belief",
		Content:    b.Content,
		Frame:      b.Frame,
		Confidence: conf,
		AssertedAt: b.AssertedAt,
		Depth:      0,
	}

	// BFS through derivation graph
	type work struct {
		id    string
		depth int
		child string // the node that cites this one
	}
	queue := []work{{id: beliefID, depth: 0}}
	visited := map[string]bool{beliefID: true}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		curNode := chain.Nodes[cur.id]
		if cur.child != "" {
			curNode.Children = append(curNode.Children, cur.child)
		}

		// Find what this node derives from
		var sources []string
		if cur.id == beliefID || curNode.Kind == "belief" {
			if bl, ok := s.beliefs[cur.id]; ok {
				sources = bl.Derivation
			}
		}
		// Records have no derivation (they're leaf nodes)

		for _, srcID := range sources {
			if !visited[srcID] {
				visited[srcID] = true
				depth := cur.depth + 1

				// Determine if this is a record or belief
				var node *ProvenanceNode
				if rec, ok := s.records[srcID]; ok {
					recConf := 1.0 // records don't have confidence per se; treat as 1.0 unless retracted
					if rec.Retracted { recConf = 0.0 }
					node = &ProvenanceNode{
						ID:           srcID,
						Kind:         "record",
						Content:      rec.Content,
						Frame:        rec.Frame,
						Confidence:   recConf,
						AssertedAt:   rec.Timestamp,
						Retracted:    rec.Retracted,
						Foundational: rec.Foundational,
						Depth:        depth,
						Parents:      nil, // records have no parents
					}
					chain.TotalRecords++
					if rec.Retracted { chain.HasRetracted = true }
				} else if bl, ok := s.beliefs[srcID]; ok {
					blFrame := s.frames[bl.Frame]
					blConf := bl.CurrentConfidence(blFrame, now)
					node = &ProvenanceNode{
						ID:         srcID,
						Kind:       "belief",
						Content:    bl.Content,
						Frame:      bl.Frame,
						Confidence: blConf,
						AssertedAt: bl.AssertedAt,
						Depth:      depth,
					}
					queue = append(queue, work{id: srcID, depth: depth, child: cur.id})
				} else {
					// Referenced ID not found — ghost node
					node = &ProvenanceNode{
						ID:      srcID,
						Kind:    "unknown",
						Content: "(not found)",
						Depth:   depth,
					}
				}
				chain.Nodes[srcID] = node
				if depth > chain.MaxDepth { chain.MaxDepth = depth }
			}
			// Register parent-child relationship
			if node, ok := chain.Nodes[srcID]; ok {
				node.Children = append(node.Children, cur.id)
			}
			// Register this source as a parent of the current node
			chain.Nodes[cur.id].Parents = append(chain.Nodes[cur.id].Parents, srcID)
		}
	}

	// Build depth-ordered list
	chain.DepthOrder = make([]string, 0, len(chain.Nodes))
	for id := range chain.Nodes { chain.DepthOrder = append(chain.DepthOrder, id) }
	sort.Slice(chain.DepthOrder, func(i, j int) bool {
		ni := chain.Nodes[chain.DepthOrder[i]]
		nj := chain.Nodes[chain.DepthOrder[j]]
		if ni.Depth != nj.Depth { return ni.Depth < nj.Depth }
		return chain.DepthOrder[i] < chain.DepthOrder[j]
	})

	return chain, nil
}

// Render returns a human-readable representation of the provenance chain.
func (c *ProvenanceChain) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Provenance chain for: %s\n", c.Root)
	fmt.Fprintf(&b, "Depth: %d  Records: %d", c.MaxDepth, c.TotalRecords)
	if c.HasRetracted { fmt.Fprintf(&b, "  ⚠ CONTAINS RETRACTED NODES") }
	fmt.Fprintf(&b, "\n\n")

	for _, id := range c.DepthOrder {
		node := c.Nodes[id]
		indent := strings.Repeat("  ", node.Depth)
		marker := "●"
		if node.Kind == "record"  { marker = "○" }
		if node.Foundational      { marker = "⚑" } // foundational records are axioms
		if node.Kind == "unknown" { marker = "?" }
		retractedMark := ""
		if node.Retracted    { retractedMark = " [RETRACTED]" }
		if node.Foundational { retractedMark += " [FOUNDATIONAL — chain terminus]" }

		content := node.Content
		if len(content) > 72 { content = content[:69] + "..." }

		fmt.Fprintf(&b, "%s%s [%s] %.0f%%  %s%s\n",
			indent, marker, node.Frame, node.Confidence*100, content, retractedMark)
		fmt.Fprintf(&b, "%s  id=%s  asserted=%s\n",
			indent, id, node.AssertedAt.Format("2006-01-02"))

		if len(node.Parents) > 0 {
			fmt.Fprintf(&b, "%s  sources: %s\n", indent, strings.Join(node.Parents, ", "))
		}
		fmt.Fprintln(&b)
	}

	return b.String()
}

// WeakestLink returns the node in the chain with the lowest confidence.
// This is the "weakest link" — retracting or revising it would most impact the chain.
// WeakestLink returns the non-root, non-foundational node with the lowest
// confidence. Foundational nodes are skipped — they are axioms, not weak
// links. Their questioning is a foundational crisis, not a chain fragility.
func (c *ProvenanceChain) WeakestLink() *ProvenanceNode {
	var weakest *ProvenanceNode
	for _, node := range c.Nodes {
		if node.ID == c.Root { continue }  // skip root
		if node.Foundational { continue }   // skip axioms
		if weakest == nil || node.Confidence < weakest.Confidence {
			weakest = node
		}
	}
	return weakest
}

// ConfidencePath returns the sequence of confidence values from root to each
// leaf record, showing how confidence evolves through the derivation chain.
// Returns paths as [][]float64, one per distinct leaf.
func (c *ProvenanceChain) ConfidencePaths() []ConfidencePath {
	var paths []ConfidencePath

	// DFS from root to leaves
	var dfs func(id string, current []PathStep)
	dfs = func(id string, current []PathStep) {
		node := c.Nodes[id]
		if node == nil { return }
		step := PathStep{ID: id, Confidence: node.Confidence, Kind: node.Kind}
		path := append(current, step)

		if len(node.Parents) == 0 || node.Kind == "record" {
			// Leaf
			cp := make([]PathStep, len(path))
			copy(cp, path)
			paths = append(paths, ConfidencePath{Steps: cp})
			return
		}
		for _, parentID := range node.Parents {
			dfs(parentID, path)
		}
	}
	dfs(c.Root, nil)
	return paths
}

// PathStep is one node in a confidence path.
type PathStep struct {
	ID         string
	Confidence float64
	Kind       string
}

// ConfidencePath is a root-to-leaf path through the derivation graph.
type ConfidencePath struct {
	Steps []PathStep
}

// MinConfidence returns the lowest confidence value along this path.
func (p ConfidencePath) MinConfidence() float64 {
	if len(p.Steps) == 0 { return 0 }
	min := p.Steps[0].Confidence
	for _, s := range p.Steps[1:] {
		if s.Confidence < min { min = s.Confidence }
	}
	return min
}
