package server

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	lumen "github.com/optakt/lumen"
)

// handleContext returns a compact belief block ready to inject into LLM context.
// Query params:
//
//	max              — max beliefs to include (default: cfg.ContextMaxBeliefs)
//	min_confidence   — minimum confidence threshold (default: cfg.ContextMinConfidence)
//	format           — "text" (default) or "json"
func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	max := s.cfg.ContextMaxBeliefs
	if v := r.URL.Query().Get("max"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			max = n
		}
	}

	minConf := s.cfg.ContextMinConfidence
	if v := r.URL.Query().Get("min_confidence"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			minConf = f
		}
	}

	type entry struct {
		id         string
		content    string
		confidence float64
		frame      string
		state      lumen.BeliefState
	}

	all := s.store.AllBeliefs(now)
	var filtered []entry
	for _, b := range all {
		if b.CurrentConfidence < minConf {
			continue
		}
		if b.State != lumen.BeliefActive && b.State != lumen.BeliefSuspect {
			continue
		}
		filtered = append(filtered, entry{
			id:         b.BeliefID,
			content:    b.Content,
			confidence: b.CurrentConfidence,
			frame:      b.Frame,
			state:      b.State,
		})
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].confidence > filtered[j].confidence
	})
	if len(filtered) > max {
		filtered = filtered[:max]
	}

	if r.URL.Query().Get("format") == "json" {
		type jsonEntry struct {
			ID         string  `json:"id"`
			Content    string  `json:"content"`
			Confidence float64 `json:"confidence"`
			Frame      string  `json:"frame"`
			Suspect    bool    `json:"suspect,omitempty"`
		}
		out := make([]jsonEntry, len(filtered))
		for i, e := range filtered {
			out[i] = jsonEntry{
				ID:         e.id,
				Content:    e.content,
				Confidence: e.confidence,
				Frame:      e.frame,
				Suspect:    e.state == lumen.BeliefSuspect,
			}
		}
		writeJSON(w, http.StatusOK, out)
		return
	}

	// Plain text — designed to drop directly into a context message.
	if len(filtered) == 0 {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "No active beliefs in store.")
		return
	}

	var sb strings.Builder
	sb.WriteString("## Current epistemic state\n\n")
	for _, e := range filtered {
		pct := int(e.confidence * 100)
		suspect := ""
		if e.state == lumen.BeliefSuspect {
			suspect = " ⚠ suspect"
		}
		sb.WriteString(fmt.Sprintf("- [%s %d%%%s] %s\n", e.frame, pct, suspect, e.content))
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, sb.String())
}
