package lumen

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// ImpactEntry describes how one belief would be affected by retracting a specific source.
type ImpactEntry struct {
	BeliefID       string
	BeliefContent  string
	CurrentConf    float64
	EstimatedConf  float64 // confidence after the source is retracted
	Drop           float64 // CurrentConf - EstimatedConf
	Distance       int     // hops from the source in the derivation graph (1 = direct)
	DirectlyLinked bool    // source is in belief\'s direct derivation list
}

func (e ImpactEntry) String() string {
	return fmt.Sprintf("[%.0f%%→%.0f%%] %s  (hop %d)", e.CurrentConf*100, e.EstimatedConf*100, e.BeliefID, e.Distance)
}

// ImpactScan computes the blast radius of retracting sourceID.
// Returns all active beliefs that would be affected, ranked by confidence drop.
//
// For directly linked beliefs: uses the fragility heuristic (remove source from
// noisy-or, scale to current confidence). For transitively linked beliefs:
// propagates the drop estimate through derivation chains.
//
// Takes a consistent snapshot under a single RLock at entry.
func (s *Store) ImpactScan(sourceID string, now time.Time) ([]ImpactEntry, error) {
	// Take a full consistent snapshot under one lock.
	s.mu.RLock()
	_, isRecord := s.records[sourceID]
	_, isBelief := s.beliefs[sourceID]
	if !isRecord && !isBelief {
		s.mu.RUnlock()
		return nil, fmt.Errorf("source %q not found (neither record nor belief)", sourceID)
	}

	// Snapshot all active beliefs with derivation and source confidences.
	type beliefSnap struct {
		id         string
		content    string
		conf       float64
		derivation []string
		sources    []sourceConf // pre-computed source confidences (excluding sourceID)
		sourcesAll []sourceConf // including sourceID
	}
	beliefs := make(map[string]*beliefSnap)
	for id, b := range s.beliefs {
		if b.State != BeliefActive { continue }
		frame := s.frames[b.Frame]
		conf := b.CurrentConfidence(frame, now)
		var srcAll, srcWithout []sourceConf
		for _, dep := range b.Derivation {
			if rec, ok := s.records[dep]; ok {
				c := 1.0
				if rec.Retracted { c = 0 }
				srcAll = append(srcAll, sourceConf{dep, "record", c})
				if dep != sourceID { srcWithout = append(srcWithout, sourceConf{dep, "record", c}) }
			} else if src, ok := s.beliefs[dep]; ok {
				srcFrame := s.frames[src.Frame]
				srcConf := src.CurrentConfidence(srcFrame, now)
				srcAll = append(srcAll, sourceConf{dep, "belief", srcConf})
				if dep != sourceID { srcWithout = append(srcWithout, sourceConf{dep, "belief", srcConf}) }
			}
		}
		beliefs[id] = &beliefSnap{
			id:         id,
			content:    b.Content,
			conf:       conf,
			derivation: append([]string{}, b.Derivation...),
			sources:    srcWithout,
			sourcesAll: srcAll,
		}
	}
	s.mu.RUnlock()

	// BFS from sourceID through derivation graph using snapshot only.
	// Queue carries: id of the affected node, hop count, and the estimated
	// confidence of that node after the cascade reaches it.
	type reach struct{ hop int; conf float64 }
	affected := make(map[string]*reach)
	// Initial entry: sourceID itself at confidence 0 (it's retracted).
	type bfsEntry struct{ id string; hop int; estimatedConf float64 }
	queue := []bfsEntry{{sourceID, 0, 0}}
	visited := map[string]bool{sourceID: true}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for id, b := range beliefs {
			if visited[id] { continue }
			links := false
			for _, dep := range b.derivation {
				if dep == cur.id { links = true; break }
			}
			if !links { continue }

			// Estimate confidence: replace cur.id in sourcesAll with the estimated
			// post-cascade confidence, then scale via noisy-or.
			adj := make([]sourceConf, 0, len(b.sourcesAll))
			for _, sc := range b.sourcesAll {
				if sc.id == cur.id {
					adj = append(adj, sourceConf{sc.id, sc.kind, cur.estimatedConf})
				} else {
					adj = append(adj, sc)
				}
			}
			estimatedConf := norScale(adj, b.sourcesAll, b.conf)
			if estimatedConf < 0 { estimatedConf = 0 }
			affected[id] = &reach{hop: cur.hop + 1, conf: estimatedConf}
			queue = append(queue, bfsEntry{id, cur.hop + 1, estimatedConf})
			visited[id] = true
		}
	}

	if len(affected) == 0 {
		return nil, nil
	}

	var entries []ImpactEntry
	for id, r := range affected {
		b := beliefs[id]
		entries = append(entries, ImpactEntry{
			BeliefID:       id,
			BeliefContent:  b.content,
			CurrentConf:    b.conf,
			EstimatedConf:  r.conf,
			Drop:           b.conf - r.conf,
			Distance:       r.hop,
			DirectlyLinked: r.hop == 1,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if math.Abs(entries[i].Drop-entries[j].Drop) > 1e-6 {
			return entries[i].Drop > entries[j].Drop
		}
		return entries[i].Distance < entries[j].Distance
	})
	return entries, nil
}

// norScale estimates confidence as noisy-or(without) / noisy-or(all) * current.
// Returns 0 if the denominator is near zero.
func norScale(without, all []sourceConf, current float64) float64 {
	if len(without) == 0 { return 0 }
	pAll := 1.0
	for _, s := range all { pAll *= (1.0 - s.conf) }
	pWithout := 1.0
	for _, s := range without { pWithout *= (1.0 - s.conf) }
	fullNOR := 1.0 - pAll
	if fullNOR < 1e-9 { return 0 }
	return (1.0 - pWithout) / fullNOR * current
}
