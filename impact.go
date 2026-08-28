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
		if b.State != BeliefActive {
			continue
		}
		frame := s.frames[b.Frame]
		conf := b.CurrentConfidence(frame, now)
		var srcAll, srcWithout []sourceConf
		for _, dep := range b.Derivation {
			if rec, ok := s.records[dep]; ok {
				c := 1.0
				if rec.Retracted {
					c = 0
				}
				srcAll = append(srcAll, sourceConf{dep, "record", c})
				if dep != sourceID {
					srcWithout = append(srcWithout, sourceConf{dep, "record", c})
				}
			} else if src, ok := s.beliefs[dep]; ok {
				srcFrame := s.frames[src.Frame]
				srcConf := src.CurrentConfidence(srcFrame, now)
				srcAll = append(srcAll, sourceConf{dep, "belief", srcConf})
				if dep != sourceID {
					srcWithout = append(srcWithout, sourceConf{dep, "belief", srcConf})
				}
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

	// Build a reverse derivation index and find the shortest distance from the
	// retracted source to every affected belief.
	dependents := make(map[string][]string)
	for id, belief := range beliefs {
		for _, source := range belief.derivation {
			dependents[source] = append(dependents[source], id)
		}
	}
	distance := map[string]int{sourceID: 0}
	queue := []string{sourceID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, dependent := range dependents[current] {
			if _, seen := distance[dependent]; seen {
				continue
			}
			distance[dependent] = distance[current] + 1
			queue = append(queue, dependent)
		}
	}

	// Recursively estimate each affected belief. Memoization ensures a diamond
	// combines every changed parent before estimating the common descendant,
	// rather than freezing the result from whichever branch the BFS saw first.
	estimated := map[string]float64{sourceID: 0}
	visiting := make(map[string]bool)
	var estimate func(string) float64
	estimate = func(id string) float64 {
		if confidence, ok := estimated[id]; ok {
			return confidence
		}
		belief, ok := beliefs[id]
		if !ok || visiting[id] {
			return 0
		}
		visiting[id] = true
		adjusted := make([]sourceConf, 0, len(belief.sourcesAll))
		for _, source := range belief.sourcesAll {
			if _, affected := distance[source.id]; affected {
				source.conf = estimate(source.id)
			}
			adjusted = append(adjusted, source)
		}
		confidence := norScale(adjusted, belief.sourcesAll, belief.conf)
		if confidence < 0 {
			confidence = 0
		}
		visiting[id] = false
		estimated[id] = confidence
		return confidence
	}

	type reach struct {
		hop  int
		conf float64
	}
	affected := make(map[string]*reach)
	for id, hop := range distance {
		if id == sourceID {
			continue
		}
		affected[id] = &reach{hop: hop, conf: estimate(id)}
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
	if len(without) == 0 {
		return 0
	}
	pAll := 1.0
	for _, s := range all {
		pAll *= (1.0 - s.conf)
	}
	pWithout := 1.0
	for _, s := range without {
		pWithout *= (1.0 - s.conf)
	}
	fullNOR := 1.0 - pAll
	if fullNOR < 1e-9 {
		return 0
	}
	return (1.0 - pWithout) / fullNOR * current
}
