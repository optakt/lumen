package lumen

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// StoreSummary provides a high-level epistemic snapshot of the store.
type StoreSummary struct {
	TotalBeliefs   int
	ActiveBeliefs  int
	SuspectBeliefs int
	Contracted     int
	TotalRecords   int
	RetractedRecs  int
	FrameStats     []FrameStat
	AvgConfidence  float64
	ConfStdDev     float64
	TopBeliefs     []ConfBelief // highest confidence
	WeakBeliefs    []ConfBelief // lowest confidence (active, non-suspect)
}

// FrameStat describes the epistemic state within one frame.
type FrameStat struct {
	Frame      string
	Count      int
	AvgConf    float64
	SuspectPct float64 // percentage that are suspect
}

// ConfBelief is a belief ID + current confidence.
type ConfBelief struct {
	ID      string
	Conf    float64
	Content string
}

// Summarize computes a store-wide epistemic summary.
func (s *Store) Summarize(now time.Time) *StoreSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sum := &StoreSummary{}

	// Records.
	for _, r := range s.records {
		sum.TotalRecords++
		if r.Retracted { sum.RetractedRecs++ }
	}

	// Beliefs by frame.
	type frameScratch struct {
		count   int
		suspect int
		confs   []float64
	}
	fmap := make(map[string]*frameScratch)

	var confs []float64
	for _, b := range s.beliefs {
		sum.TotalBeliefs++
		switch b.State {
		case BeliefActive:
			sum.ActiveBeliefs++
		case BeliefSuspect:
			sum.SuspectBeliefs++
		case BeliefSuperseded:
			sum.Contracted++
			continue // exclude from stats
		}
		frame := s.frames[b.Frame]
		c := b.CurrentConfidence(frame, now)
		confs = append(confs, c)

		if _, ok := fmap[b.Frame]; !ok {
			fmap[b.Frame] = &frameScratch{}
		}
		fs := fmap[b.Frame]
		fs.count++
		fs.confs = append(fs.confs, c)
		if b.State == BeliefSuspect { fs.suspect++ }
	}

	// Global confidence stats.
	if len(confs) > 0 {
		total := 0.0
		for _, c := range confs { total += c }
		sum.AvgConfidence = total / float64(len(confs))
		variance := 0.0
		for _, c := range confs {
			d := c - sum.AvgConfidence
			variance += d * d
		}
		sum.ConfStdDev = math.Sqrt(variance / float64(len(confs)))
	}

	// Frame stats.
	for name, fs := range fmap {
		avg := 0.0
		for _, c := range fs.confs { avg += c }
		if fs.count > 0 { avg /= float64(fs.count) }
		sum.FrameStats = append(sum.FrameStats, FrameStat{
			Frame:      name,
			Count:      fs.count,
			AvgConf:    avg,
			SuspectPct: float64(fs.suspect) / math.Max(float64(fs.count), 1) * 100,
		})
	}
	sort.Slice(sum.FrameStats, func(i, j int) bool { return sum.FrameStats[i].Frame < sum.FrameStats[j].Frame })

	// Top/weak beliefs.
	type entry struct{ id, content string; conf float64; state BeliefState }
	var all []entry
	for id, b := range s.beliefs {
		if b.State == BeliefSuperseded { continue }
		frame := s.frames[b.Frame]
		all = append(all, entry{id, b.Content, b.CurrentConfidence(frame, now), b.State})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].conf > all[j].conf })
	top := 5
	if top > len(all) { top = len(all) }
	for _, e := range all[:top] {
		sum.TopBeliefs = append(sum.TopBeliefs, ConfBelief{e.id, e.conf, e.content})
	}
	// Weak: lowest active (non-suspect) beliefs.
	var active []entry
	for _, e := range all {
		if e.state == BeliefActive { active = append(active, e) }
	}
	sort.Slice(active, func(i, j int) bool { return active[i].conf < active[j].conf })
	weak := 5
	if weak > len(active) { weak = len(active) }
	for _, e := range active[:weak] {
		sum.WeakBeliefs = append(sum.WeakBeliefs, ConfBelief{e.id, e.conf, e.content})
	}

	return sum
}

// RenderSummary returns a human-readable store summary.
func RenderSummary(s *StoreSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Store summary\n\n")
	fmt.Fprintf(&b, "  Beliefs:  %d total  (%d active, %d suspect, %d contracted)\n",
		s.TotalBeliefs, s.ActiveBeliefs, s.SuspectBeliefs, s.Contracted)
	fmt.Fprintf(&b, "  Records:  %d total  (%d retracted)\n", s.TotalRecords, s.RetractedRecs)
	if s.ActiveBeliefs+s.SuspectBeliefs > 0 {
		fmt.Fprintf(&b, "  Avg confidence: %.0f%%  (σ=%.0f%%)\n",
			s.AvgConfidence*100, s.ConfStdDev*100)
	}
	if len(s.FrameStats) > 0 {
		fmt.Fprintf(&b, "\n  By frame:\n")
		for _, fs := range s.FrameStats {
			suspectNote := ""
			if fs.SuspectPct > 0 {
				suspectNote = fmt.Sprintf("  %.0f%% suspect", fs.SuspectPct)
			}
			fmt.Fprintf(&b, "    %-15s %3d beliefs  avg %.0f%%%s\n",
				fs.Frame, fs.Count, fs.AvgConf*100, suspectNote)
		}
	}
	if len(s.TopBeliefs) > 0 {
		fmt.Fprintf(&b, "\n  Highest confidence:\n")
		for _, bel := range s.TopBeliefs {
			c := bel.Content
			if len(c) > 55 { c = c[:52] + "..." }
			fmt.Fprintf(&b, "    %.0f%%  %-25s  %q\n", bel.Conf*100, bel.ID, c)
		}
	}
	if len(s.WeakBeliefs) > 0 {
		fmt.Fprintf(&b, "\n  Lowest active confidence:\n")
		for _, bel := range s.WeakBeliefs {
			c := bel.Content
			if len(c) > 55 { c = c[:52] + "..." }
			fmt.Fprintf(&b, "    %.0f%%  %-25s  %q\n", bel.Conf*100, bel.ID, c)
		}
	}
	return b.String()
}
