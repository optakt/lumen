package lumen

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// TemporalGraph tracks assertion ordering and temporal dependencies between
// beliefs and records in the belief store.
//
// This is the fourth graph in Lumen's four-graph architecture:
//  1. BeliefGraph    — derivation and semantic relationships
//  2. EntityGraph    — named entity co-mention
//  3. BridgeRegistry — frame-to-frame translation protocols
//  4. TemporalGraph  — assertion ordering and historical state
//
// The temporal graph enables counterfactual queries:
//
//	"What did the store believe about X before record R was asserted?"
//	"Which beliefs could not have existed without record R?"
//	"What was the earliest time at which belief B was supportable?"
type TemporalGraph struct {
	mu sync.RWMutex
	// events is the ordered list of assertion events, chronologically.
	// Stored as pointers so byID references remain valid after slice shifts on insert.
	events []*TemporalEvent
	// byID indexes events by node ID for fast lookup.
	byID map[string]*TemporalEvent
}

// TemporalEvent records when a node was asserted and what it enabled.
type TemporalEvent struct {
	NodeID     string
	Kind       string // "record" or "belief"
	AssertedAt time.Time
	// EnabledBy is the set of source IDs whose prior assertion was required
	// for this node to be assertable (i.e., its derivation sources).
	EnabledBy []string
}

func NewTemporalGraph() *TemporalGraph {
	return &TemporalGraph{
		byID: make(map[string]*TemporalEvent),
	}
}

// Record registers an assertion event in the temporal graph.
func (g *TemporalGraph) Record(nodeID, kind string, assertedAt time.Time, enabledBy []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ev := &TemporalEvent{
		NodeID:     nodeID,
		Kind:       kind,
		AssertedAt: assertedAt,
		EnabledBy:  append([]string{}, enabledBy...),
	}
	// Insert in chronological order. The slice holds *TemporalEvent so byID
	// pointers remain valid regardless of append reallocation or element shifting.
	idx := sort.Search(len(g.events), func(i int) bool {
		return g.events[i].AssertedAt.After(assertedAt)
	})
	g.events = append(g.events, nil)
	copy(g.events[idx+1:], g.events[idx:])
	g.events[idx] = ev
	g.byID[nodeID] = ev
}

// StateAt returns the set of node IDs that existed in the store at or before
// the given time — i.e., had been asserted by that point.
func (g *TemporalGraph) StateAt(t time.Time) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var result []string
	for _, ev := range g.events {
		if !ev.AssertedAt.After(t) {
			result = append(result, ev.NodeID)
		}
	}
	return result
}

// EnabledBy returns which previously-asserted nodes were required for nodeID to exist.
func (g *TemporalGraph) EnabledBy(nodeID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ev, ok := g.byID[nodeID]
	if !ok {
		return nil
	}
	return append([]string{}, ev.EnabledBy...)
}

// WouldExistWithout answers: would nodeID exist if sourceID had never been asserted?
// Returns false if nodeID's derivation includes sourceID, or if nodeID was asserted
// after sourceID and lists it as an enabler.
func (g *TemporalGraph) WouldExistWithout(nodeID, sourceID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ev, ok := g.byID[nodeID]
	if !ok {
		return false // node doesn't exist
	}
	for _, dep := range ev.EnabledBy {
		if dep == sourceID {
			return false
		}
	}
	return true
}

// CounterfactualRemoval returns the set of nodes that would NOT exist
// if sourceID had never been asserted. This is the temporal analog of
// MinimalContraction: it operates on the assertion history, not just
// the current derivation graph.
//
// Algorithm: BFS/DFS over EnabledBy edges from sourceID. A node cannot
// exist if it is enabled (directly or transitively) by sourceID alone.
func (g *TemporalGraph) CounterfactualRemoval(sourceID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Build reverse enablement index: who depends on whom
	dependents := make(map[string][]string) // sourceID → list of nodes it enables
	for _, ev := range g.events {
		for _, dep := range ev.EnabledBy {
			dependents[dep] = append(dependents[dep], ev.NodeID)
		}
	}

	// BFS from sourceID through dependents
	visited := make(map[string]bool)
	queue := []string{sourceID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, dep := range dependents[cur] {
			if !visited[dep] {
				// Check if this dependent has ANY other enabler not in visited set
				// If all its enablers are in visited, it too must be removed
				ev := g.byID[dep]
				if ev == nil {
					continue
				}
				allEnablersRemoved := true
				for _, en := range ev.EnabledBy {
					if !visited[en] && en != sourceID {
						allEnablersRemoved = false
						break
					}
				}
				if len(ev.EnabledBy) == 0 || allEnablersRemoved {
					visited[dep] = true
					queue = append(queue, dep)
				}
			}
		}
	}

	result := make([]string, 0, len(visited))
	for id := range visited {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

// Timeline returns all events in chronological order.
func (g *TemporalGraph) Timeline() []TemporalEvent {
	g.mu.RLock()
	defer g.mu.RUnlock()
	result := make([]TemporalEvent, len(g.events))
	for i, ev := range g.events {
		result[i] = *ev
		result[i].EnabledBy = append([]string(nil), ev.EnabledBy...)
	}
	return result
}

// EarliestSupportableAt returns the earliest time at which beliefID could have
// been asserted — i.e., the latest AssertedAt among all its direct enablers.
// A belief cannot logically precede the evidence it derives from.
func (g *TemporalGraph) EarliestSupportableAt(beliefID string) (time.Time, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ev, ok := g.byID[beliefID]
	if !ok {
		return time.Time{}, fmt.Errorf("node %s not found in temporal graph", beliefID)
	}
	if len(ev.EnabledBy) == 0 {
		return ev.AssertedAt, nil // no dependencies; can be asserted anytime
	}
	var latest time.Time
	for _, dep := range ev.EnabledBy {
		depEv, ok := g.byID[dep]
		if !ok {
			continue
		}
		if depEv.AssertedAt.After(latest) {
			latest = depEv.AssertedAt
		}
	}
	return latest, nil
}

// Remove removes a node from the temporal graph.
// Used by ApplyContraction to prevent temporal queries returning dead IDs.
func (g *TemporalGraph) Remove(nodeID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// Remove from byID index.
	delete(g.byID, nodeID)
	// Remove from events slice.
	newEvents := g.events[:0]
	for _, ev := range g.events {
		if ev.NodeID != nodeID {
			newEvents = append(newEvents, ev)
		}
	}
	g.events = newEvents
	// Remove nodeID from EnabledBy lists of other events.
	for _, ev := range g.events {
		newEnabled := ev.EnabledBy[:0]
		for _, id := range ev.EnabledBy {
			if id != nodeID {
				newEnabled = append(newEnabled, id)
			}
		}
		ev.EnabledBy = newEnabled
	}
}
