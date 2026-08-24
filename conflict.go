package lumen

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ConflictDetector scans a belief store for potential epistemic conflicts —
// pairs of beliefs that may assert incompatible claims.
//
// Without semantic understanding, exact contradiction detection is impossible.
// ConflictDetector uses three heuristic signals:
//
//  1. Negation proximity — one belief's content contains explicit negation
//     markers ("not", "no", "cannot", "fails to", "does not") relative to
//     keywords shared with another belief. High co-mention + negation = likely conflict.
//
//  2. Confidence divergence — two beliefs sharing entities where one is very
//     high confidence and the other very low. Asymmetric confidence about the
//     same subject is a weak conflict signal.
//
//  3. Contrastive edges — beliefs explicitly linked via Graph.Contrast() edges.
//     These are declared conflicts, not inferred ones.
//
// Results are ranked by conflict strength (0.0–1.0) and include an explanation
// of why each pair was flagged.

// Conflict represents a potential epistemic conflict between two beliefs.
type Conflict struct {
	BeliefA    string
	BeliefB    string
	Strength   float64 // 0.0 = weak signal, 1.0 = strong signal
	Kind       string  // "negation", "divergence", "declared"
	Explanation string
}

// ConflictScan scans the store for potential conflicts and returns them
// sorted by strength descending.
//
// Results are cached and only recomputed when the belief set has changed
// since the last call. The cache is invalidated by any write to beliefs or
// records. This keeps the call O(1) after the first scan in steady state,
// which matters because BeliefHealth and StoreHealth both call it.
func (s *Store) ConflictScan(now time.Time) []Conflict {
	s.mu.RLock()
	if !s.conflictDirty {
		cached := make([]Conflict, len(s.conflictCache))
		copy(cached, s.conflictCache)
		s.mu.RUnlock()
		return cached
	}
	s.mu.RUnlock()

	// Recompute outside the lock (snapshot approach): this avoids holding
	// the lock across the O(n²) scan. A concurrent write may invalidate
	// again before we store the result; we accept that (the result is still
	// correct for the snapshot we took).
	s.mu.RLock()
	// Snapshot belief IDs and contents for analysis outside the lock
	type snap struct {
		id         string
		content    string
		frame      string
		confidence float64
	}
	var beliefs []snap
	for id, b := range s.beliefs {
		if b.State == BeliefSuperseded { continue } // skip contracted beliefs
		frame := s.frames[b.Frame]
		beliefs = append(beliefs, snap{
			id:         id,
			content:    b.Content,
			frame:      b.Frame,
			confidence: b.CurrentConfidence(frame, now),
		})
	}
	// Snapshot contrast edges
	contrastPairs := s.Graph.ContrastEdges()
	s.mu.RUnlock()

	var conflicts []Conflict

	// Signal 1: declared contrast edges
	for _, pair := range contrastPairs {
		conflicts = append(conflicts, Conflict{
			BeliefA:     pair[0],
			BeliefB:     pair[1],
			Strength:    1.0,
			Kind:        "declared",
			Explanation: fmt.Sprintf("Beliefs %q and %q are explicitly linked as contrasting views.", pair[0], pair[1]),
		})
	}

	// Signals 2 & 3: entity-based checks — negation proximity and confidence divergence.
	//
	// Old approach: O(n²) pair loop with CoMentionedBetween (2 lock acquisitions +
	// set intersection) in each inner body — O(n² × e × locks).
	//
	// New approach: snapshot the entity→nodes inverted index once, then iterate over
	// entities. For each entity, only the O(k²) pairs among the k beliefs that mention
	// it need to be considered. Because k << n in practice, the total work is O(entities × k²)
	// instead of O(n²). The co-mention map is built in the same pass, eliminating the
	// per-pair CoMentionedBetween call entirely.
	negationMarkers := []string{
		"not ", "no ", "cannot ", "fails to ", "does not ", "is not ",
		"are not ", "neither ", "without ", "lack", "absent", "impossible",
		"unconfirmed", "refuted", "rejected", "inconsistent",
	}

	// Build belief lookup for O(1) snap access.
	belief := make(map[string]int, len(beliefs))
	for i, b := range beliefs {
		belief[b.id] = i
	}

	// Inverted index snapshot: entity → belief node IDs that mention it.
	entityNodes := s.Entities.EntitySnapshot()

	// pairShared accumulates shared-entity names per ordered pair (a < b lexicographically).
	type pairKey [2]string
	pairShared := make(map[pairKey][]string)
	for entityID, nodes := range entityNodes {
		// Only belief nodes (those present in our snapshot) are relevant.
		var bids []string
		for _, nid := range nodes {
			if _, ok := belief[nid]; ok {
				bids = append(bids, nid)
			}
		}
		if len(bids) < 2 {
			continue // no pair can form from a single node
		}
		for i := 0; i < len(bids); i++ {
			for j := i + 1; j < len(bids); j++ {
				a, b := bids[i], bids[j]
				if a > b { a, b = b, a } // canonical order
				pairShared[pairKey{a, b}] = append(pairShared[pairKey{a, b}], entityID)
			}
		}
	}

	// Declared-conflict set for deduplication.
	type pairSet map[pairKey]bool
	alreadyCaught := make(pairSet)
	for _, c := range conflicts {
		a, b := c.BeliefA, c.BeliefB
		if a > b { a, b = b, a }
		alreadyCaught[pairKey{a, b}] = true
	}

	for pk, co := range pairShared {
		// Recover snapped belief data via the index built above.
		ai, aok := belief[pk[0]]
		bi, bok := belief[pk[1]]
		if !aok || !bok { continue }
		a, b := beliefs[ai], beliefs[bi]

		aLower := strings.ToLower(a.content)
		bLower := strings.ToLower(b.content)
		aNegates := containsNegationOf(aLower, bLower, negationMarkers)
		bNegates := containsNegationOf(bLower, aLower, negationMarkers)

		if aNegates || bNegates {
			strength := 0.6 + 0.2*float64(len(co))
			if strength > 0.95 { strength = 0.95 }
			direction := a.id + " negates aspects of " + b.id
			if bNegates { direction = b.id + " negates aspects of " + a.id }
			conflicts = append(conflicts, Conflict{
				BeliefA: a.id, BeliefB: b.id, Strength: strength, Kind: "negation",
				Explanation: fmt.Sprintf("%s. Shared entities: %s.", direction, strings.Join(co, ", ")),
			})
			alreadyCaught[pk] = true
		}

		if !alreadyCaught[pk] {
			diff := math.Abs(a.confidence - b.confidence)
			if diff >= 0.5 && (a.confidence >= 0.8 || b.confidence >= 0.8) {
				strength := 0.3 + 0.3*(diff-0.5)/0.5
				conflicts = append(conflicts, Conflict{
					BeliefA: a.id, BeliefB: b.id, Strength: strength, Kind: "divergence",
					Explanation: fmt.Sprintf(
						"Confidence divergence of %.0f%% on shared entities (%s): %s=%.0f%%, %s=%.0f%%.",
						diff*100, strings.Join(co, ", "),
						a.id, a.confidence*100, b.id, b.confidence*100,
					),
				})
			}
		}
	}

	// Sort by strength descending
	sort.Slice(conflicts, func(i, j int) bool {
		return conflicts[i].Strength > conflicts[j].Strength
	})

	// Cache the result.
	s.mu.Lock()
	s.conflictCache = conflicts
	s.conflictDirty = false
	s.mu.Unlock()

	return conflicts
}

// invalidateConflicts marks the conflict cache as dirty.
// Must be called under s.mu.Lock() by any write path that changes beliefs or
// records (confidence changes, additions, retractions, contractions).
func (s *Store) invalidateConflicts() {
	s.conflictDirty = true
}

// containsNegationOf checks whether text A contains a negation marker
// adjacent to keywords also found in text B.
func containsNegationOf(a, b string, markers []string) bool {
	// Extract significant words from b (ignore stop words)
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"is": true, "are": true, "was": true, "were": true, "in": true,
		"of": true, "to": true, "it": true, "that": true, "this": true,
		"with": true, "for": true, "as": true, "at": true, "by": true,
	}
	words := strings.Fields(b)
	var keywords []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?()")
		if len(w) > 4 && !stopWords[w] {
			keywords = append(keywords, w)
		}
	}

	for _, marker := range markers {
		if !strings.Contains(a, marker) {
			continue
		}
		// Check if a negation marker appears near a keyword from b
		idx := strings.Index(a, marker)
		for idx != -1 {
			window := a[idx : minInt(idx+60, len(a))]
			for _, kw := range keywords {
				if strings.Contains(window, kw) {
					return true
				}
			}
			nextIdx := strings.Index(a[idx+1:], marker)
			if nextIdx == -1 { break }
			idx = idx + 1 + nextIdx
		}
	}
	return false
}

func minInt(a, b int) int {
	if a < b { return a }
	return b
}

// ContrastEdges returns all declared contrast edge pairs from the belief graph.
func (g *BeliefGraph) ContrastEdges() [][2]string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var pairs [][2]string
	for _, e := range g.edges {
		if e.Kind == EdgeContrasts {
			pairs = append(pairs, [2]string{e.From, e.To})
		}
	}
	return pairs
}

// CoMentionedBetween returns entities shared between two specific nodes.
func (eg *EntityGraph) CoMentionedBetween(nodeA, nodeB string) []string {
	entitiesA := eg.EntitiesForNode(nodeA)
	entitiesB := eg.EntitiesForNode(nodeB)
	setA := make(map[string]bool, len(entitiesA))
	for _, e := range entitiesA {
		setA[e] = true
	}
	var shared []string
	for _, e := range entitiesB {
		if setA[e] {
			shared = append(shared, e)
		}
	}
	sort.Strings(shared)
	return shared
}


