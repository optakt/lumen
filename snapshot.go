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
	frames := make(map[string]Frame, len(s.frames))
	records := make(map[string]*Record, len(s.records))
	beliefs := make(map[string]*Belief, len(s.beliefs))
	queries := make(map[string]ParsedQuery, len(s.namedQueries))
	recordVersions := make(map[string][]RecordVersion, len(s.recordVersions))
	for id, frame := range s.frames {
		frames[id] = frame
	}
	for id, record := range s.records {
		records[id] = cloneRecord(record)
	}
	for id, belief := range s.beliefs {
		beliefs[id] = cloneBelief(belief)
	}
	for id, query := range s.namedQueries {
		queries[id] = query
	}
	for id, history := range s.recordVersions {
		recordVersions[id] = cloneRecordVersions(history)
	}

	s.versions.mu.RLock()
	versions := make(map[string][]BeliefVersion, len(s.versions.versions))
	for id, history := range s.versions.versions {
		versions[id] = cloneVersions(history)
	}
	s.versions.mu.RUnlock()
	bridgeList := s.Bridges.All()
	timeline := s.Temporal.Timeline()
	s.mu.RUnlock()

	existsAt := make(map[string]bool)
	for _, event := range timeline {
		if !event.AssertedAt.After(t) {
			existsAt[event.NodeID] = true
		}
	}

	// Restore the belief state immediately before the first change after t.
	// If no later change exists, the current state is already the state at t.
	for id, belief := range beliefs {
		for _, version := range versions[id] {
			if t.Before(version.ChangedAt) {
				applyBeliefVersion(belief, version)
				break
			}
		}
	}

	snap := NewStore()
	snap.frames = frames
	snap.namedQueries = queries
	for _, bridge := range bridgeList {
		copyBridge := *bridge
		_ = snap.Bridges.Register(&copyBridge)
	}

	for id, record := range records {
		if !existsAt[id] {
			continue
		}
		for _, version := range recordVersions[id] {
			if t.Before(version.ChangedAt) {
				record = cloneRecord(&version.Record)
				break
			}
		}
		if record.Retracted && !record.RetractedAt.IsZero() && t.Before(record.RetractedAt) {
			record.Retracted = false
			record.RetractedAt = time.Time{}
			record.RetractReason = ""
		}
		snap.records[id] = record
		snap.Temporal.Record(id, "record", record.Timestamp, nil)
	}

	admitted := make(map[string]bool, len(snap.records)+len(beliefs))
	for id := range snap.records {
		admitted[id] = true
	}
	for _, event := range timeline {
		if event.Kind != "belief" || event.AssertedAt.After(t) || admitted[event.NodeID] {
			continue
		}
		belief, ok := beliefs[event.NodeID]
		if !ok {
			continue
		}
		sourcesPresent := true
		for _, sourceID := range belief.Derivation {
			if !admitted[sourceID] {
				sourcesPresent = false
				break
			}
		}
		if !sourcesPresent {
			continue
		}
		snap.beliefs[belief.ID] = belief
		admitted[belief.ID] = true
		for _, sourceID := range belief.Derivation {
			if snap.dependents[sourceID] == nil {
				snap.dependents[sourceID] = make(map[string]bool)
			}
			snap.dependents[sourceID][belief.ID] = true
		}
		snap.Temporal.Record(belief.ID, "belief", belief.AssertedAt, belief.Derivation)
		if history, ok := versions[belief.ID]; ok {
			snap.versions.versions[belief.ID] = cloneVersions(history)
		}
	}

	snap.Graph = s.Graph.CloneFiltered(admitted)
	// Rebuild derivation edges from historical belief state. The current graph
	// may no longer contain edges removed by a later contraction.
	for _, belief := range snap.beliefs {
		for _, sourceID := range belief.Derivation {
			snap.Graph.AddEdge(Edge{From: sourceID, To: belief.ID, Kind: EdgeDerives})
		}
	}
	snap.Entities = s.Entities.CloneFiltered(admitted)
	snap.conflictDirty = true
	snap.searchDirty = true
	return snap
}

func applyBeliefVersion(belief *Belief, version BeliefVersion) {
	belief.Content = version.Content
	belief.Confidence = version.Confidence
	belief.Frame = version.Frame
	belief.State = version.State
	belief.Derivation = append([]string(nil), version.Derivation...)
	belief.ContractedBy = version.ContractedBy
	belief.ImportedDecay = append([]DecayPolicy(nil), version.ImportedDecay...)
	belief.CrossFrame = append([]CrossFrameSource(nil), version.CrossFrame...)
	belief.CompositionPrior = version.CompositionPrior
	belief.CompositionEvidence = append([]Evidence(nil), version.CompositionEvidence...)
	belief.AssertedAt = version.AssertedAt
	if version.DecayOverride == nil {
		belief.DecayOverride = nil
	} else {
		copyDecay := *version.DecayOverride
		belief.DecayOverride = &copyDecay
	}
}
