package lumen

import (
	"fmt"
	"sync"
	"time"
)

// Store is the belief store: an append-only record log + mutable belief graph.
type Store struct {
	mu      sync.RWMutex
	frames  map[string]Frame
	records map[string]*Record
	beliefs map[string]*Belief
	// derivation index: beliefID -> set of beliefIDs that depend on it
	dependents map[string]map[string]bool
	// conflictCache caches the result of ConflictScan. It is invalidated
	// by any mutation that could change the conflict graph. Guarded by mu.
	conflictCache []Conflict
	conflictDirty bool
	// searchIndex caches the TF-IDF search index. Rebuilt on the first query
	// after any write. Guarded by mu.
	searchIndex  *SearchIndex
	searchDirty  bool
	// Graph holds the full typed relationship structure.
	// derivation edges here supersede the dependents map (kept for compatibility).
	Graph *BeliefGraph
	// Entities tracks the bipartite graph between content nodes and named entities.
	Entities *EntityGraph
	// Bridges holds declared frame-to-frame translation protocols.
	Bridges *BridgeRegistry
	// Temporal tracks assertion ordering and historical state.
	Temporal *TemporalGraph
	// versions holds the version history for each belief.
	versions *VersionStore
	// namedQueries holds queries declared in .lm files, keyed by query ID.
	// Not persisted to BoltDB; re-loaded from the source file on startup.
	namedQueries map[string]ParsedQuery
}

func NewStore() *Store {
	return &Store{
		frames:     make(map[string]Frame),
		records:    make(map[string]*Record),
		beliefs:    make(map[string]*Belief),
		dependents: make(map[string]map[string]bool),
		conflictDirty: true,
		searchDirty:  true,
		Graph:      NewBeliefGraph(),
		Entities:   NewEntityGraph(),
		Bridges:    NewBridgeRegistry(),
		Temporal:   NewTemporalGraph(),
		versions:     NewVersionStore(),
		namedQueries:  make(map[string]ParsedQuery),
	}
}

// RegisterFrame adds a frame to the store.
func (s *Store) RegisterFrame(f Frame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames[f.Name] = f
}

// Assert adds a record to the store. Records are immutable once added.
func (s *Store) Assert(r *Record) error {
	s.mu.Lock()
	if _, exists := s.records[r.ID]; exists {
		s.mu.Unlock()
		return fmt.Errorf("record %s already exists", r.ID)
	}
	if _, exists := s.beliefs[r.ID]; exists {
		s.mu.Unlock()
		return fmt.Errorf("ID %s already used by a belief", r.ID)
	}
	if _, ok := s.frames[r.Frame]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("unknown frame %s", r.Frame)
	}
	s.records[r.ID] = r
	s.invalidateConflicts()
	s.invalidateSearch()
	s.mu.Unlock()
	// Index entities and record temporal event after releasing store lock.
	s.Entities.ExtractAndIndex(r.ID, r.Content)
	s.Temporal.Record(r.ID, "record", r.Timestamp, nil)
	return nil
}

// Reference adds a non-inferential semantic edge from `fromID` to `toID`.
// This records that fromID is epistemically related to toID (responds to, is about,
// contrasts with) without creating a retraction dependency.
func (s *Store) Reference(fromID, toID string, kind EdgeKind, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Both nodes must exist in the store
	if _, ok := s.beliefs[fromID]; !ok {
		if _, ok := s.records[fromID]; !ok {
			return fmt.Errorf("source node %s not found", fromID)
		}
	}
	if _, ok := s.beliefs[toID]; !ok {
		if _, ok := s.records[toID]; !ok {
			return fmt.Errorf("target node %s not found", toID)
		}
	}
	if kind == EdgeDerives {
		return fmt.Errorf("use Believe() to add derivation edges, not Reference()")
	}
	s.Graph.AddEdge(Edge{From: fromID, To: toID, Kind: kind, Label: label})
	return nil
}

// Believe adds a belief to the store, tracking its derivation dependencies.
//
// Lock ordering: s.mu is held for the full operation including graph/dependent
// registration. Sub-structure mutexes (Graph.mu, Temporal.mu, Entities.mu)
// are always acquired after s.mu, never the reverse. Do not release s.mu
// mid-operation to call into sub-structures; they are safe to call under s.mu.
// Entity/temporal indexing runs after s.mu is released because it is cheap,
// owns only its own mutex, and brief inconsistency (belief in store, not yet
// in entity index) is acceptable for a single-writer REPL. If the store is
// used concurrently, move those calls under the lock.
func (s *Store) Believe(b *Belief) error {
	// Validate before taking the lock so we can return early cleanly.
	if b.ID == "" {
		return fmt.Errorf("belief ID must not be empty")
	}

	s.mu.Lock()

	// Enforce one ID namespace: beliefs, records, and beliefsV2 may not share IDs.
	if _, exists := s.beliefs[b.ID]; exists {
		s.mu.Unlock()
		return fmt.Errorf("belief %s already exists", b.ID)
	}
	if _, exists := s.records[b.ID]; exists {
		s.mu.Unlock()
		return fmt.Errorf("ID %s already used by a record", b.ID)
	}
	if _, ok := s.frames[b.Frame]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("unknown frame %s", b.Frame)
	}

	// Check derivation sources exist and collect imported decay policies.
	var imported []DecayPolicy
	for _, srcID := range b.Derivation {
		if rec, ok := s.records[srcID]; ok {
			if rec.Retracted {
				b.State = BeliefSuspect
			}
			if s.dependents[srcID] == nil {
				s.dependents[srcID] = make(map[string]bool)
			}
			s.dependents[srcID][b.ID] = true
			s.Graph.AddEdge(Edge{From: srcID, To: b.ID, Kind: EdgeDerives})
		} else if src, ok := s.beliefs[srcID]; ok {
			if src.State == BeliefSuspect {
				b.State = BeliefSuspect
			}
			srcFrame := s.frames[src.Frame]
			if src.Frame != b.Frame {
				// Cross-frame source: capture snapshot of source confidence at assertion
				// time (fixes the retrodiction problem). The receiving frame's own decay
				// applies going forward; the source frame's ongoing decay does not.
				snapConf := src.CurrentConfidence(srcFrame, b.AssertedAt)
				b.CrossFrame = append(b.CrossFrame, CrossFrameSource{
					SourceBeliefID:     srcID,
					SourceFrame:        src.Frame,
					ConfidenceAtImport: snapConf,
					ImportedAt:         b.AssertedAt,
				})
				// Keep ImportedDecay for legacy callers; CrossFrame takes precedence
				// in CurrentConfidence when populated.
				policy := srcFrame.Decay
				if src.DecayOverride != nil {
					policy = *src.DecayOverride
				}
				imported = append(imported, policy)
				imported = append(imported, src.ImportedDecay...)
			}
			if s.dependents[srcID] == nil {
				s.dependents[srcID] = make(map[string]bool)
			}
			s.dependents[srcID][b.ID] = true
			s.Graph.AddEdge(Edge{From: srcID, To: b.ID, Kind: EdgeDerives})
		} else {
			s.mu.Unlock()
			return fmt.Errorf("derivation source %s not found", srcID)
		}
	}
	if len(imported) > 0 {
		b.ImportedDecay = append(b.ImportedDecay, imported...)
	}
	s.beliefs[b.ID] = b
	s.invalidateConflicts()
	s.invalidateSearch()
	derivation := append([]string{}, b.Derivation...)
	s.mu.Unlock()

	// Entity/temporal indexing: runs after lock release; see note above.
	s.Entities.ExtractAndIndex(b.ID, b.Content)
	s.Temporal.Record(b.ID, "belief", b.AssertedAt, derivation)
	return nil
}

// Retract poisons a record and marks all dependent beliefs as suspect.
func (s *Store) Retract(recordID string, reason string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[recordID]
	if !ok {
		return fmt.Errorf("record %s not found", recordID)
	}
	rec.Retracted = true
	rec.RetractedAt = at
	rec.RetractReason = reason
	// Mark all beliefs derived from this record as suspect (recursive)
	s.markSuspect(recordID)
	s.invalidateConflicts()
	return nil
}

// markSuspect marks all beliefs reachable from id via derivation edges as suspect.
// Uses BeliefGraph as the single source of truth for the derivation relation —
// dependents is kept for now but markSuspect no longer relies on it, so the two
// cannot diverge after load.
// Must be called with s.mu held (Graph.ReachableByDerivation takes g.mu internally).
func (s *Store) markSuspect(id string) {
	reachable := s.Graph.ReachableByDerivation(id)
	for _, depID := range reachable {
		if b, ok := s.beliefs[depID]; ok {
			b.State = BeliefSuspect
		}
	}
}

// QueryResult is what you get back from a belief query.
type QueryResult struct {
	BeliefID          string
	Content           string
	CurrentConfidence float64
	State             BeliefState
	AssertedAt        time.Time
	Frame             string
	DecayedSince      time.Duration
}

// Query returns the current state of a belief, with decay applied.
func (s *Store) Query(beliefID string, now time.Time) (*QueryResult, error) {
	// First read: check the frame's staleness policy.
	s.mu.RLock()
	b, ok := s.beliefs[beliefID]
	if !ok {
		s.mu.RUnlock()
		return nil, fmt.Errorf("belief %s not found", beliefID)
	}
	frame, fok := s.frames[b.Frame]
	s.mu.RUnlock()
	if !fok {
		return nil, fmt.Errorf("frame %s not found", b.Frame)
	}

	// Apply on_stale_derivation policy — may acquire write lock internally.
	if err := s.applyOnStaleDerivation(beliefID, frame, now); err != nil {
		return nil, err
	}

	// Second read: return the (possibly-updated) state.
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok = s.beliefs[beliefID]
	if !ok {
		return nil, fmt.Errorf("belief %s not found", beliefID)
	}
	frame = s.frames[b.Frame]
	return &QueryResult{
		BeliefID:          b.ID,
		Content:           b.Content,
		CurrentConfidence: b.CurrentConfidence(frame, now),
		State:             b.State,
		AssertedAt:        b.AssertedAt,
		Frame:             b.Frame,
		DecayedSince:      now.Sub(b.AssertedAt),
	}, nil
}

// AllBeliefs returns current confidence for every belief in the store.
func (s *Store) AllBeliefs(now time.Time) []*QueryResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	results := make([]*QueryResult, 0, len(s.beliefs))
	for id := range s.beliefs {
		b := s.beliefs[id]
		frame := s.frames[b.Frame]
		results = append(results, &QueryResult{
			BeliefID:          b.ID,
			Content:           b.Content,
			CurrentConfidence: b.CurrentConfidence(frame, now),
			State:             b.State,
			AssertedAt:        b.AssertedAt,
			Frame:             b.Frame,
			DecayedSince:      now.Sub(b.AssertedAt),
		})
	}
	return results
}





// BelieveComposed adds a belief with full Bayesian composition metadata.
// It computes the posterior from prior and evidence, compares it to the
// declared confidence, and warns if the discrepancy exceeds the threshold.
// The belief is stored regardless of calibration warnings — the store does
// not refuse to record beliefs, only audits them.
func (s *Store) BelieveComposed(b *Belief, prior float64, evidence []Evidence) (*ComposedBelief, error) {
	// Opaque frames do not support evidence decomposition.
	s.mu.RLock()
	frame, fok := s.frames[b.Frame]
	s.mu.RUnlock()
	if fok && frame.IsOpaque() {
		opacityNote := ""
		if frame.Calibration != "" {
			opacityNote = fmt.Sprintf("; use calibration method %q instead", frame.Calibration)
		}
		return nil, fmt.Errorf("frame %q is opaque — Bayesian composition not available%s", b.Frame, opacityNote)
	}

	// Validate calibration first — composition errors abort before storage.
	cb, err := ValidateConfidence(b.Confidence, prior, evidence)
	if err != nil {
		return nil, fmt.Errorf("composition: %w", err)
	}
	cb.ID = b.ID
	cb.Content = b.Content
	cb.Frame = b.Frame

	// Store composition metadata on the belief so it survives persistence
	// and is available to FragilityScan for sensitivity analysis.
	b.CompositionPrior    = prior
	b.CompositionEvidence = evidence

	// Delegate storage to Believe: one path owns graph edges, cross-frame
	// snapshots, suspect inheritance, and entity/temporal indexing.
	// Calibration is advisory, not blocking — the belief is stored as declared.
	if err := s.Believe(b); err != nil {
		return nil, err
	}
	return cb, nil
}

// ContentFor returns the content string for any node ID (record or belief).
// Returns the ID itself if not found.
func (s *Store) ContentFor(id string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if rec, ok := s.records[id]; ok {
		return rec.Content
	}
	if b, ok := s.beliefs[id]; ok {
		return b.Content
	}
	return id
}

// RecordCount returns the number of records in the store.
func (s *Store) RecordCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}
