package lumen

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ExportJSON exports the store as a JSON document.
// The output is human-readable and includes all beliefs, records,
// their current confidence, state, and metadata.
func (s *Store) ExportJSON(now time.Time) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type recordExport struct {
		ID            string    `json:"id"`
		Frame         string    `json:"frame"`
		Content       string    `json:"content"`
		Timestamp     time.Time `json:"timestamp"`
		Retracted     bool      `json:"retracted,omitempty"`
		RetractReason string    `json:"retract_reason,omitempty"`
		Foundational  bool      `json:"foundational,omitempty"`
	}
	type beliefExport struct {
		ID           string    `json:"id"`
		Frame        string    `json:"frame"`
		Content      string    `json:"content"`
		Confidence   float64   `json:"confidence"`
		Current      float64   `json:"current_confidence"`
		State        string    `json:"state"`
		AssertedAt   time.Time `json:"asserted_at"`
		Derivation   []string  `json:"derivation,omitempty"`
		ContractedBy string    `json:"contracted_by,omitempty"`
	}
	type storeExport struct {
		ExportedAt time.Time      `json:"exported_at"`
		Records    []recordExport `json:"records"`
		Beliefs    []beliefExport `json:"beliefs"`
	}

	out := storeExport{ExportedAt: now}

	// Records sorted by timestamp
	var recs []*Record
	for _, r := range s.records { recs = append(recs, r) }
	sort.Slice(recs, func(i, j int) bool { return recs[i].Timestamp.Before(recs[j].Timestamp) })
	for _, r := range recs {
		out.Records = append(out.Records, recordExport{
			ID:            r.ID,
			Frame:         r.Frame,
			Content:       r.Content,
			Timestamp:     r.Timestamp,
			Retracted:     r.Retracted,
			RetractReason: r.RetractReason,
			Foundational:  r.Foundational,
		})
	}

	// Beliefs sorted by confidence descending
	var bels []*Belief
	for _, b := range s.beliefs { bels = append(bels, b) }
	sort.Slice(bels, func(i, j int) bool {
		fi := s.frames[bels[i].Frame]
		fj := s.frames[bels[j].Frame]
		return bels[i].CurrentConfidence(fi, now) > bels[j].CurrentConfidence(fj, now)
	})
	for _, b := range bels {
		frame := s.frames[b.Frame]
		out.Beliefs = append(out.Beliefs, beliefExport{
			ID:           b.ID,
			Frame:        b.Frame,
			Content:      b.Content,
			Confidence:   b.Confidence,
			Current:      b.CurrentConfidence(frame, now),
			State:        stateToString(b.State),
			AssertedAt:   b.AssertedAt,
			Derivation:   b.Derivation,
			ContractedBy: b.ContractedBy,
		})
	}

	return json.MarshalIndent(out, "", "  ")
}

// ExportMarkdown exports the store as a Markdown document.
// Useful for reading, sharing, or publishing belief store contents.
func (s *Store) ExportMarkdown(title string, now time.Time) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "*Exported %s*\n\n", now.Format("2006-01-02"))
	fmt.Fprintf(&b, "%d beliefs, %d records\n\n", len(s.beliefs), len(s.records))

	// Group beliefs by frame (skip contracted/superseded)
	byFrame := make(map[string][]*Belief)
	for _, bel := range s.beliefs {
		if bel.State == BeliefSuperseded { continue }
		byFrame[bel.Frame] = append(byFrame[bel.Frame], bel)
	}
	var frames []string
	for f := range byFrame { frames = append(frames, f) }
	sort.Strings(frames)

	for _, frame := range frames {
		bels := byFrame[frame]
		sort.Slice(bels, func(i, j int) bool {
			fi := s.frames[bels[i].Frame]
			fj := s.frames[bels[j].Frame]
			return bels[i].CurrentConfidence(fi, now) > bels[j].CurrentConfidence(fj, now)
		})
		titleFrame := frame
		if len(titleFrame) > 0 {
			titleFrame = strings.ToUpper(titleFrame[:1]) + titleFrame[1:]
		}
		fmt.Fprintf(&b, "## %s\n\n", titleFrame)
		for _, bel := range bels {
			f := s.frames[bel.Frame]
			conf := bel.CurrentConfidence(f, now)
			stateStr := ""
			if bel.State == BeliefSuspect { stateStr = " ⚠ SUSPECT" }
			fmt.Fprintf(&b, "**%.0f%%** — %s%s\n\n", conf*100, bel.Content, stateStr)
			fmt.Fprintf(&b, "> `%s`", bel.ID)
			if len(bel.Derivation) > 0 {
				fmt.Fprintf(&b, " — sources: %s", strings.Join(bel.Derivation, ", "))
			}
			fmt.Fprintf(&b, "\n\n")
		}
	}

	// Records (collapsed)
	if len(s.records) > 0 {
		fmt.Fprintf(&b, "## Records\n\n")
		var recs []*Record
		for _, r := range s.records { recs = append(recs, r) }
		sort.Slice(recs, func(i, j int) bool { return recs[i].Timestamp.Before(recs[j].Timestamp) })
		for _, r := range recs {
			retracted := ""
			if r.Retracted { retracted = " ~~retracted~~" }
			foundational := ""
			if r.Foundational { foundational = " ⚑" }
			fmt.Fprintf(&b, "- `%s`%s (%s)%s  %s\n", r.ID, foundational, r.Timestamp.Format("2006-01-02"), retracted, r.Content)
		}
		fmt.Fprintln(&b)
	}


	return b.String()
}

// ExportLM exports the store as a .lm file.
// This produces a file that can be loaded back with LoadFile.
// ExportLM produces a valid .lm file from the current store state.
// The output can be reloaded with LoadFile to reproduce the same beliefs.
func (s *Store) ExportLM(now time.Time) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var b strings.Builder
	fmt.Fprintf(&b, "// Exported %s\n", now.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "// %d records, %d active beliefs\n\n", len(s.records), len(s.beliefs))

	// Frames sorted by name
	var frames []Frame
	for _, f := range s.frames { frames = append(frames, f) }
	sort.Slice(frames, func(i, j int) bool { return frames[i].Name < frames[j].Name })
	if len(frames) > 0 {
		for _, f := range frames {
			fmt.Fprintf(&b, "frame %s\n", f.Name)
			if f.IsOpaque() {
				fmt.Fprintf(&b, "    composition: opaque\n")
				if f.OpaqueSource != "" { fmt.Fprintf(&b, "    source: %q\n", f.OpaqueSource) }
				if f.Calibration != "" { fmt.Fprintf(&b, "    calibration: %s\n", f.Calibration) }
				if f.OpaqueReason != "" { fmt.Fprintf(&b, "    opacity-reason: %q\n", f.OpaqueReason) }
			} else if f.Composition != CompositionBayesian {
				fmt.Fprintf(&b, "    composition: %s\n", f.Composition)
			}
			switch f.Decay.Kind {
			case DecayNone:
				fmt.Fprintf(&b, "    decay: none\n")
			case DecayExponential:
				days := f.Decay.Halflife.Hours() / 24
				fmt.Fprintf(&b, "    decay: exponential halflife: %.0fd\n", days)
			}
			if f.OnStaleDerivation != StaleIgnore {
				fmt.Fprintf(&b, "    on_stale_derivation: %s\n", f.OnStaleDerivation)
			}
			fmt.Fprintln(&b)
		}
	}

	// Records sorted by timestamp
	var recs []*Record
	for _, r := range s.records { recs = append(recs, r) }
	sort.Slice(recs, func(i, j int) bool { return recs[i].Timestamp.Before(recs[j].Timestamp) })

	if len(recs) > 0 {
		for _, r := range recs {
			if r.Retracted {
				fmt.Fprintf(&b, "// RETRACTED record %s in %s\n", r.ID, r.Frame)
				fmt.Fprintf(&b, "//   reason: %s\n\n", r.RetractReason)
				continue
			}
			fmt.Fprintf(&b, "record %s in %s\n", r.ID, r.Frame)
			// Content must be a single-line quoted string.
			content := strings.ReplaceAll(r.Content, "\n", " ")
			fmt.Fprintf(&b, "    %q\n", content)
			if !r.Timestamp.IsZero() {
				fmt.Fprintf(&b, "    at: %q\n", r.Timestamp.UTC().Format(time.RFC3339))
			}
			if r.Foundational {
				fmt.Fprintf(&b, "    provenance: foundational\n")
			}
			fmt.Fprintln(&b)
		}
	}

	// Beliefs sorted by ID (active only — superseded are soft-deleted)
	var bels []*Belief
	for _, b2 := range s.beliefs {
		if b2.State != BeliefSuperseded {
			bels = append(bels, b2)
		}
	}
	sort.Slice(bels, func(i, j int) bool { return bels[i].ID < bels[j].ID })

	if len(bels) > 0 {
		for _, bel := range bels {
			fmt.Fprintf(&b, "believe %s in %s\n", bel.ID, bel.Frame)
			content := strings.ReplaceAll(bel.Content, "\n", " ")
			fmt.Fprintf(&b, "    %q\n", content)
			fmt.Fprintf(&b, "    confidence: %.4f\n", bel.Confidence)
			if len(bel.Derivation) > 0 {
				fmt.Fprintf(&b, "    from: %s\n", strings.Join(bel.Derivation, ", "))
			}
			fmt.Fprintln(&b)
		}
	}

	// Named queries sorted by ID.
	all := s.AllQueries()
	if len(all) > 0 {
		for _, q := range all {
			fmt.Fprintf(&b, "query %s\n", q.ID)
			fmt.Fprintf(&b, "    target: %s\n", q.Target)
			fmt.Fprintf(&b, "    select: %s\n", q.Select)
			if q.Since != "" {
				fmt.Fprintf(&b, "    since: %q\n", q.Since)
			}
			if q.Where != "" {
				fmt.Fprintf(&b, "    where: %s\n", q.Where)
			}
			fmt.Fprintln(&b)
		}
	}

	return b.String()
}