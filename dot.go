package lumen

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// DotOptions controls the DOT export.
type DotOptions struct {
	// IncludeRetracted includes retracted records (dashed border).
	IncludeRetracted bool
	// IncludeSuperseded includes soft-deleted contracted beliefs.
	IncludeSuperseded bool
	// ColorByFrame uses a distinct fill color per frame.
	ColorByFrame bool
	// Now is used to compute decayed confidences.
	Now time.Time
}

// DefaultDotOptions returns sensible defaults.
func DefaultDotOptions(now time.Time) DotOptions {
	return DotOptions{ColorByFrame: true, Now: now}
}

var frameColors = []string{
	"#d4e8ff", "#d4ffd4", "#ffd4d4", "#ffffd4", "#f0d4ff",
	"#ffd4f0", "#d4fff0", "#fff0d4", "#e8e8e8", "#d4d4ff",
}

// ExportDot returns a Graphviz DOT representation of the belief graph.
//
// Records appear as rectangle nodes, beliefs as ellipses. Edges show
// derivation. Node color reflects confidence (green=high, red=low).
// Suspect beliefs are shown with a dashed border.
func (s *Store) ExportDot(opts DotOptions) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}

	// Assign a color to each frame.
	frameIdx := make(map[string]int)
	for name := range s.frames {
		idx := len(frameIdx) % len(frameColors)
		frameIdx[name] = idx
	}

	var b strings.Builder
	b.WriteString("digraph lumen {\n")
	b.WriteString("  rankdir=BT;\n") // beliefs flow up from records
	b.WriteString("  node [fontname=\"Helvetica\" fontsize=11];\n")
	b.WriteString("  edge [color=\"#666666\"];\n\n")

	// Records.
	b.WriteString("  // Records\n")
	for id, rec := range s.records {
		if rec.Retracted && !opts.IncludeRetracted {
			continue
		}
		label := dotTruncate(rec.Content, 35)
		color := "#ffffff"
		if opts.ColorByFrame {
			if idx, ok := frameIdx[rec.Frame]; ok {
				color = frameColors[idx]
			}
		}
		style := ""
		if rec.Retracted {
			style = ` style="dashed"`
		} else if rec.Foundational {
			style = ` style="bold"`
		}
		fmt.Fprintf(&b, "  %q [shape=box label=%q fillcolor=%q style=\"filled%s\" tooltip=%q];\n",
			id, id+"\n"+label, color, style, rec.Frame+": "+dotTruncate(rec.Content, 50))
	}
	b.WriteString("\n")

	// Beliefs.
	b.WriteString("  // Beliefs\n")
	for id, bel := range s.beliefs {
		if bel.State == BeliefSuperseded && !opts.IncludeSuperseded {
			continue
		}
		frame := s.frames[bel.Frame]
		conf := bel.CurrentConfidence(frame, opts.Now)
		label := fmt.Sprintf("%s\n%.0f%%", id, conf*100)
		color := confidenceColor(conf)
		border := "solid"
		if bel.State == BeliefSuspect {
			border = "dashed"
		} else if bel.State == BeliefSuperseded {
			border = "dotted"
		}
		frameColor := "#ffffff"
		if opts.ColorByFrame {
			if idx, ok := frameIdx[bel.Frame]; ok {
				frameColor = frameColors[idx]
			}
		}
		_ = frameColor
		fmt.Fprintf(&b, "  %q [shape=ellipse label=%q fillcolor=%q style=\"filled,%s\" tooltip=%q];\n",
			id, label, color, border, bel.Frame+": "+dotTruncate(bel.Content, 50))
	}
	b.WriteString("\n")

	// Edges (derivation).
	b.WriteString("  // Derivation edges\n")
	for id, bel := range s.beliefs {
		if bel.State == BeliefSuperseded && !opts.IncludeSuperseded {
			continue
		}
		for _, dep := range bel.Derivation {
			fmt.Fprintf(&b, "  %q -> %q;\n", dep, id)
		}
	}

	// Bridge edges (declared).
	if brs := s.Bridges.All(); len(brs) > 0 {
		b.WriteString("\n  // Bridges\n")
		for _, br := range brs {
			fmt.Fprintf(&b, "  // bridge %s: %s → %s\n", br.Name, br.FromFrame, br.ToFrame)
		}
	}

	b.WriteString("}\n")
	return b.String()
}

// confidenceColor maps confidence [0,1] to a green-yellow-red fill color.
//
//   1.0 → #aaf0aa (light green)
//   0.7 → #f5f5a0 (light yellow)
//   0.4 → #f5c060 (amber)
//   0.0 → #f08080 (light red)
func confidenceColor(conf float64) string {
	conf = math.Max(0, math.Min(1, conf))
	// Interpolate through three colour stops: red→amber→yellow→green
	// using two linear segments.
	var r, g, b int
	if conf >= 0.7 {
		f := (conf - 0.7) / 0.3 // 0→1 as conf goes 0.7→1.0
		r = lerp(245, 170, f)    // #f5 → #aa
		g = lerp(245, 240, f)    // #f5 → #f0
		b = lerp(160, 170, f)    // #a0 → #aa
	} else if conf >= 0.4 {
		f := (conf - 0.4) / 0.3 // 0→1 as conf goes 0.4→0.7
		r = 245                  // constant red channel
		g = lerp(192, 245, f)   // #c0 → #f5
		b = lerp( 96, 160, f)   // #60 → #a0
	} else {
		f := conf / 0.4          // 0→1 as conf goes 0→0.4
		r = lerp(240, 245, f)   // #f0 → #f5
		g = lerp(128, 192, f)   // #80 → #c0
		b = lerp(128,  96, f)   // #80 → #60
	}
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func lerp(a, b int, t float64) int {
	v := float64(a) + t*float64(b-a)
	if v < 0 { v = 0 }
	if v > 255 { v = 255 }
	return int(v)
}

func dotTruncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n { return s }
	return s[:n-3] + "..."
}
