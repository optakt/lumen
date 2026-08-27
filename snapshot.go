package lumen

import "time"

// SnapshotAt returns a read-only view of the store as it stood at time t.
//
// The snapshot contains only nodes whose assertion timestamp is at or before t,
// and whose derivation sources all existed at t. All four graphs are rebuilt
// over the filtered set. Frames and bridges are carried over in full — they
// are configuration, not evidence, and do not have assertion timestamps.
//
// The returned store is safe to query with any existing Store method
// (Query, Explain, FragilityScan, etc.). Do not call Assert, Believe, or
// Retract on a snapshot: the graphs will diverge from the originating store
// and the snapshot has no BoltDB backing.
//
// Example:
//
//	snap := store.SnapshotAt(time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC))
//	result, _ := snap.Query("hard-problem", time.Now())
//	// result reflects what the store believed before 2022 about that belief.
func (s *Store) SnapshotAt(t time.Time) *Store {
	s.mu.RLock()

	// Collect the IDs of all nodes asserted at or before t.
	// TemporalGraph.StateAt acquires its own lock, so release ours first.
	frames  := make(map[string]Frame, len(s.frames))
	records := make(map[string]*Record, len(s.records))
	beliefs := make(map[string]*Belief, len(s.beliefs))

	for k, v := range s.frames  { frames[k]  = v }
	for k, v := range s.records { records[k] = v }
	for k, v := range s.beliefs { beliefs[k] = v }

	// Copy bridges.
	bridgeList := s.Bridges.All()

	s.mu.RUnlock()

	// Ask the temporal graph which nodes existed at t.
	existsAt := make(map[string]bool)
	for _, id := range s.Temporal.StateAt(t) {
		existsAt[id] = true
	}

	// Build the snapshot store.
	snap := NewStore()

	// Register all frames — they are configuration, not timestamped evidence.
	for _, f := range frames {
		snap.RegisterFrame(f)
	}

	// Register all bridges.
	for _, br := range bridgeList {
		snap.Bridges.Register(br)
	}

	// Admit records that existed at t.
	for id, r := range records {
		if !existsAt[id] {
			continue
		}
		// Deep-copy so the snapshot is independent.
		rc := *r
		_ = snap.Assert(&rc) //nolint:errcheck // only fails on duplicate ID, impossible here
	}

	// Admit beliefs that existed at t and whose derivation sources all existed at t.
	// Process in temporal order so derivation sources are always admitted before
	// their dependents.
	timeline := s.Temporal.Timeline()
	for _, ev := range timeline {
		if ev.Kind != "belief" {
			continue
		}
		if !existsAt[ev.NodeID] {
			continue
		}
		b, ok := beliefs[ev.NodeID]
		if !ok {
			continue
		}
		// Require all derivation sources to exist in the snapshot.
		sourcesPresent := true
		for _, srcID := range b.Derivation {
			if !existsAt[srcID] {
				sourcesPresent = false
				break
			}
		}
		if !sourcesPresent {
			continue
		}
		bc := *b
		bc.CrossFrame   = append([]CrossFrameSource{}, b.CrossFrame...)
		bc.ImportedDecay = append([]DecayPolicy{}, b.ImportedDecay...)
		bc.Derivation    = append([]string{}, b.Derivation...)
		bc.CompositionEvidence = append([]Evidence{}, b.CompositionEvidence...)
		_ = snap.Believe(&bc) //nolint:errcheck // only fails on duplicate ID, impossible here
	}

	return snap
}
