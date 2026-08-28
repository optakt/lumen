package lumen

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Persist provides durable storage for a Lumen belief store using BoltDB.
// Each record and belief is stored as a JSON-encoded value under its ID.
// The store can be saved at any point and fully restored on next load.
//
// Design: we serialize the minimal representation of each record and belief —
// enough to reconstruct the in-memory store exactly. Graph edges are derived
// from the Derivation fields on beliefs; entity mentions and temporal events
// are re-indexed on load. Bridge declarations are stored separately.
//
// Buckets:
//   frames   — frame definitions
//   records  — Record structs
//   beliefs  — Belief structs
//   bridges  — Bridge structs

const (
	bucketFrames         = "frames"
	bucketRecords        = "records"
	bucketBeliefs        = "beliefs"
	bucketBeliefsV2      = "beliefsv2"
	bucketBridges        = "bridges"
	bucketVersions       = "versions"
	bucketEntities       = "entities"
	bucketEdges          = "edges"
	bucketTemporal       = "temporal"
	bucketRecordVersions = "record-versions"
)

// OpenDB opens (or creates) a BoltDB database at the given path.
func OpenDB(path string) (*bolt.DB, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return db, db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{
			bucketFrames, bucketRecords, bucketBeliefs, bucketBeliefsV2,
			bucketBridges, bucketVersions, bucketEntities, bucketEdges, bucketTemporal,
			bucketRecordVersions,
		} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	})
}

// SaveStore persists the entire store state to a BoltDB database.
// Existing entries with the same IDs are overwritten.
func SaveStore(s *Store, db *bolt.DB) error {
	s.mu.RLock()
	// Deep-copy under the read lock so encoding sees one consistent state.
	frames := make(map[string]Frame, len(s.frames))
	records := make(map[string]*Record, len(s.records))
	beliefs := make(map[string]*Belief, len(s.beliefs))
	recordVersions := make(map[string][]RecordVersion, len(s.recordVersions))
	for k, v := range s.frames {
		frames[k] = v
	}
	for k, v := range s.records {
		records[k] = cloneRecord(v)
	}
	for k, v := range s.beliefs {
		beliefs[k] = cloneBelief(v)
	}
	for id, history := range s.recordVersions {
		recordVersions[id] = cloneRecordVersions(history)
	}
	s.mu.RUnlock()

	all := s.Bridges.All()
	bridges := make(map[string]*Bridge, len(all))
	for _, bridge := range all {
		copyBridge := *bridge
		bridges[copyBridge.Name] = &copyBridge
	}

	s.versions.mu.RLock()
	versions := make(map[string][]BeliefVersion, len(s.versions.versions))
	for beliefID, history := range s.versions.versions {
		versions[beliefID] = cloneVersions(history)
	}
	s.versions.mu.RUnlock()
	entityState := s.Entities.snapshot()
	edges := s.Graph.AllEdges()
	timeline := s.Temporal.Timeline()

	return db.Update(func(tx *bolt.Tx) error {
		if err := putJSON(tx, bucketFrames, frames); err != nil {
			return err
		}
		if err := putEach(tx, bucketRecords, records); err != nil {
			return err
		}
		if err := putEach(tx, bucketBeliefs, beliefs); err != nil {
			return err
		}
		if err := putEach(tx, bucketRecordVersions, recordVersions); err != nil {
			return err
		}
		if err := putEach(tx, bucketBridges, bridges); err != nil {
			return err
		}
		if err := putJSON(tx, bucketEntities, entityState); err != nil {
			return err
		}
		if err := putJSON(tx, bucketEdges, edges); err != nil {
			return err
		}
		if err := putJSON(tx, bucketTemporal, timeline); err != nil {
			return err
		}

		// Save version history
		vb := tx.Bucket([]byte(bucketVersions))
		if err := clearBucket(vb); err != nil {
			return err
		}
		for beliefID, history := range versions {
			data, err := json.Marshal(history)
			if err != nil {
				return err
			}
			if err := vb.Put([]byte(beliefID), data); err != nil {
				return err
			}
		}
		return nil
	})
}

// LoadStore restores a store from a BoltDB database.
// The store must have no prior state (call NewStore() first).
// After loading, graph edges, entity mentions, and temporal events
// are re-derived from the restored beliefs and records.
func LoadStore(db *bolt.DB, now time.Time) (*Store, error) {
	s := NewStore()

	return s, db.View(func(tx *bolt.Tx) error {
		// Frames first
		var frames map[string]Frame
		if err := getJSON(tx, bucketFrames, &frames); err != nil {
			return fmt.Errorf("load frames: %w", err)
		}
		for _, f := range frames {
			s.RegisterFrame(f)
		}

		// Records
		rb := tx.Bucket([]byte(bucketRecords))
		if rb != nil {
			if err := rb.ForEach(func(k, v []byte) error {
				var r Record
				if err := json.Unmarshal(v, &r); err != nil {
					return err
				}
				// Restore directly without re-asserting (skip validation)
				s.mu.Lock()
				s.records[r.ID] = &r
				s.mu.Unlock()
				// Re-index entities and temporal
				s.Entities.ExtractAndIndex(r.ID, r.Content)
				s.Temporal.Record(r.ID, "record", r.Timestamp, nil)
				return nil
			}); err != nil {
				return fmt.Errorf("load records: %w", err)
			}
		}

		// Record versions (pre-revision/retraction states for snapshots).
		rvb := tx.Bucket([]byte(bucketRecordVersions))
		if rvb != nil {
			if err := rvb.ForEach(func(k, v []byte) error {
				var history []RecordVersion
				if err := json.Unmarshal(v, &history); err != nil {
					return err
				}
				s.mu.Lock()
				s.recordVersions[string(k)] = history
				s.mu.Unlock()
				return nil
			}); err != nil {
				return fmt.Errorf("load record versions: %w", err)
			}
		}

		// Beliefs
		bb := tx.Bucket([]byte(bucketBeliefs))
		if bb != nil {
			if err := bb.ForEach(func(k, v []byte) error {
				var b Belief
				if err := json.Unmarshal(v, &b); err != nil {
					return err
				}
				s.mu.Lock()
				s.beliefs[b.ID] = &b
				// Re-wire graph edges
				for _, srcID := range b.Derivation {
					s.Graph.AddEdge(Edge{From: srcID, To: b.ID, Kind: EdgeDerives})
				}
				s.mu.Unlock()
				s.Entities.ExtractAndIndex(b.ID, b.Content)
				s.Temporal.Record(b.ID, "belief", b.AssertedAt, b.Derivation)
				return nil
			}); err != nil {
				return fmt.Errorf("load beliefs: %w", err)
			}
		}

		// Bridges
		brb := tx.Bucket([]byte(bucketBridges))
		if brb != nil {
			if err := brb.ForEach(func(k, v []byte) error {
				var br Bridge
				if err := json.Unmarshal(v, &br); err != nil {
					return err
				}
				s.Bridges.Register(&br)
				return nil
			}); err != nil {
				return fmt.Errorf("load bridges: %w", err)
			}
		}

		// Restore derived graph state that cannot be reconstructed from records
		// and beliefs alone: registered entities, semantic edges, and the full
		// temporal event stream. Older databases omit these buckets and fall back
		// to the indexes reconstructed above.
		var entityState entityGraphState
		if err := getJSON(tx, bucketEntities, &entityState); err != nil {
			return fmt.Errorf("load entities: %w", err)
		}
		if len(entityState.Entities) > 0 || len(entityState.Mentions) > 0 {
			s.Entities = entityGraphFromState(entityState)
		}

		var edges []Edge
		if err := getJSON(tx, bucketEdges, &edges); err != nil {
			return fmt.Errorf("load edges: %w", err)
		}
		for _, edge := range edges {
			s.Graph.AddEdge(edge)
		}

		var timeline []TemporalEvent
		if err := getJSON(tx, bucketTemporal, &timeline); err != nil {
			return fmt.Errorf("load temporal graph: %w", err)
		}
		if len(timeline) > 0 {
			s.Temporal = NewTemporalGraph()
			for _, event := range timeline {
				s.Temporal.Record(event.NodeID, event.Kind, event.AssertedAt, event.EnabledBy)
			}
		}

		// Rebuild dependents from Graph so cascade is consistent after load.
		// markSuspect now uses Graph directly, but dependents is kept as a
		// secondary index for Believe's registration path.
		s.mu.Lock()
		// Walk all derivation edges in the graph and reconstruct dependents.
		for beliefID, b := range s.beliefs {
			for _, srcID := range b.Derivation {
				if s.dependents[srcID] == nil {
					s.dependents[srcID] = make(map[string]bool)
				}
				s.dependents[srcID][beliefID] = true
			}
		}
		s.mu.Unlock()

		// Versions
		vb := tx.Bucket([]byte(bucketVersions))
		if vb != nil {
			if err := vb.ForEach(func(k, v []byte) error {
				var versions []BeliefVersion
				if err := json.Unmarshal(v, &versions); err != nil {
					return err
				}
				s.versions.mu.Lock()
				s.versions.versions[string(k)] = versions
				s.versions.mu.Unlock()
				return nil
			}); err != nil {
				return fmt.Errorf("load versions: %w", err)
			}
		}

		return nil
	})
}

// helpers

func putJSON(tx *bolt.Tx, bucket string, v interface{}) error {
	b := tx.Bucket([]byte(bucket))
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return b.Put([]byte("_all"), data)
}

func getJSON(tx *bolt.Tx, bucket string, v interface{}) error {
	b := tx.Bucket([]byte(bucket))
	if b == nil {
		return nil
	}
	data := b.Get([]byte("_all"))
	if data == nil {
		return nil
	}
	return json.Unmarshal(data, v)
}

func putEach[T any](tx *bolt.Tx, bucket string, m map[string]T) error {
	b := tx.Bucket([]byte(bucket))
	if err := clearBucket(b); err != nil {
		return err
	}
	for k, v := range m {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if err := b.Put([]byte(k), data); err != nil {
			return err
		}
	}
	return nil
}

func clearBucket(b *bolt.Bucket) error {
	var keys [][]byte
	if err := b.ForEach(func(k, _ []byte) error {
		keys = append(keys, append([]byte(nil), k...))
		return nil
	}); err != nil {
		return err
	}
	for _, key := range keys {
		if err := b.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

func cloneRecord(r *Record) *Record {
	copyRecord := *r
	copyRecord.Provenance.Sources = append([]string(nil), r.Provenance.Sources...)
	return &copyRecord
}

func cloneBelief(b *Belief) *Belief {
	copyBelief := *b
	copyBelief.Provenance.Sources = append([]string(nil), b.Provenance.Sources...)
	copyBelief.Derivation = append([]string(nil), b.Derivation...)
	copyBelief.ImportedDecay = append([]DecayPolicy(nil), b.ImportedDecay...)
	copyBelief.CrossFrame = append([]CrossFrameSource(nil), b.CrossFrame...)
	copyBelief.CompositionEvidence = append([]Evidence(nil), b.CompositionEvidence...)
	if b.DecayOverride != nil {
		copyDecay := *b.DecayOverride
		copyBelief.DecayOverride = &copyDecay
	}
	return &copyBelief
}

func cloneVersions(history []BeliefVersion) []BeliefVersion {
	result := make([]BeliefVersion, len(history))
	for i := range history {
		result[i] = cloneBeliefVersion(history[i])
	}
	return result
}
