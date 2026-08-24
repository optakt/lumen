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
	bucketFrames    = "frames"
	bucketRecords   = "records"
	bucketBeliefs   = "beliefs"
	bucketBeliefsV2 = "beliefsv2"
	bucketBridges   = "bridges"
	bucketVersions  = "versions"
)

// OpenDB opens (or creates) a BoltDB database at the given path.
func OpenDB(path string) (*bolt.DB, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return db, db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{bucketFrames, bucketRecords, bucketBeliefs, bucketBeliefsV2, bucketBridges, bucketVersions} {
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
	// Snapshot all data under the read lock
	frames    := make(map[string]Frame, len(s.frames))
	records   := make(map[string]*Record, len(s.records))
	beliefs   := make(map[string]*Belief, len(s.beliefs))
	for k, v := range s.frames    { frames[k]    = v }
	for k, v := range s.records   { records[k]   = v }
	for k, v := range s.beliefs   { beliefs[k]   = v }
	s.mu.RUnlock()


	return db.Update(func(tx *bolt.Tx) error {
		if err := putJSON(tx, bucketFrames, frames); err != nil { return err }
		if err := putEach(tx, bucketRecords, records); err != nil { return err }
		if err := putEach(tx, bucketBeliefs, beliefs); err != nil { return err }
	// Collect bridges from registry
		s.Bridges.mu.RLock()
		all := s.Bridges.All()
		bridges := make(map[string]*Bridge, len(all))
		for _, v := range all { bridges[v.Name] = v }
		s.Bridges.mu.RUnlock()
		if err := putEach(tx, bucketBridges, bridges); err != nil { return err }

		// Save version history
		s.versions.mu.RLock()
		for beliefID, versions := range s.versions.versions {
			data, err := json.Marshal(versions)
			if err != nil { s.versions.mu.RUnlock(); return err }
			if err := tx.Bucket([]byte(bucketVersions)).Put([]byte(beliefID), data); err != nil {
				s.versions.mu.RUnlock()
				return err
			}
		}
		s.versions.mu.RUnlock()
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
				if err := json.Unmarshal(v, &r); err != nil { return err }
				// Restore directly without re-asserting (skip validation)
				s.mu.Lock()
				s.records[r.ID] = &r
				s.mu.Unlock()
				// Re-index entities and temporal
				s.Entities.ExtractAndIndex(r.ID, r.Content)
				s.Temporal.Record(r.ID, "record", r.Timestamp, nil)
				return nil
			}); err != nil { return fmt.Errorf("load records: %w", err) }
		}

		// Beliefs
		bb := tx.Bucket([]byte(bucketBeliefs))
		if bb != nil {
			if err := bb.ForEach(func(k, v []byte) error {
				var b Belief
				if err := json.Unmarshal(v, &b); err != nil { return err }
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
			}); err != nil { return fmt.Errorf("load beliefs: %w", err) }
		}

		// Bridges
		brb := tx.Bucket([]byte(bucketBridges))
		if brb != nil {
			if err := brb.ForEach(func(k, v []byte) error {
				var br Bridge
				if err := json.Unmarshal(v, &br); err != nil { return err }
				s.Bridges.Register(&br)
				return nil
			}); err != nil { return fmt.Errorf("load bridges: %w", err) }
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
				if err := json.Unmarshal(v, &versions); err != nil { return err }
				s.versions.mu.Lock()
				s.versions.versions[string(k)] = versions
				s.versions.mu.Unlock()
				return nil
			}); err != nil { return fmt.Errorf("load versions: %w", err) }
		}

		return nil
	})
}

// helpers

func putJSON(tx *bolt.Tx, bucket string, v interface{}) error {
	b := tx.Bucket([]byte(bucket))
	data, err := json.Marshal(v)
	if err != nil { return err }
	return b.Put([]byte("_all"), data)
}

func getJSON(tx *bolt.Tx, bucket string, v interface{}) error {
	b := tx.Bucket([]byte(bucket))
	if b == nil { return nil }
	data := b.Get([]byte("_all"))
	if data == nil { return nil }
	return json.Unmarshal(data, v)
}

func putEach[T any](tx *bolt.Tx, bucket string, m map[string]T) error {
	b := tx.Bucket([]byte(bucket))
	for k, v := range m {
		data, err := json.Marshal(v)
		if err != nil { return err }
		if err := b.Put([]byte(k), data); err != nil { return err }
	}
	return nil
}

func bridgeMap(bridges []*Bridge) map[string]*Bridge {
	m := make(map[string]*Bridge, len(bridges))
	for _, br := range bridges {
		m[br.Name] = br
	}
	return m
}
