package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	lumen "github.com/optakt/lumen"
	"github.com/optakt/lumen/self"
)

// selfState is persisted across requests via the server's belief store.
// Claims are stored as regular beliefs with a "self:" ID prefix; corrections
// are recorded as retractions. This reuses all of Lumen's existing persistence
// without a separate BoltDB bucket.

// ─── Routes (called from routes()) ───────────────────────────────────────────

func (s *Server) routeSelf() {
	// Versioned self-model routes.
	s.mux.HandleFunc("POST /v1/self/claim",              s.handleSelfClaim)
	s.mux.HandleFunc("POST /v1/self/correct",            s.handleSelfCorrect)
	s.mux.HandleFunc("GET /v1/self/claims",              s.handleSelfList)
	s.mux.HandleFunc("GET /v1/self/context",             s.handleSelfContext)
	s.mux.HandleFunc("GET /v1/self/biography/{id}",      s.handleSelfBiography)
	s.mux.HandleFunc("GET /v1/self/frame-report",        s.handleSelfFrameReport)

	// Legacy redirects for self-model routes — 308 with dynamic target,
	// see redirectToV1 in server.go for the rationale.
	for _, from := range []string{
		"POST /self/claim",
		"POST /self/correct",
		"GET /self/claims",
		"GET /self/context",
		"GET /self/biography/{id}",
		"GET /self/frame-report",
	} {
		s.mux.HandleFunc(from, redirectToV1)
	}
}

// ─── Handlers ────────────────────────────────────────────────────────────────

type claimRequest struct {
	ID         string   `json:"id,omitempty"`
	Kind       string   `json:"kind"` // asserted | derived | retrieved | corrected
	Content    string   `json:"content"`
	Confidence float64  `json:"confidence,omitempty"`
	Frame      string   `json:"frame,omitempty"`
	Sources    []string `json:"sources,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

func (s *Server) handleSelfClaim(w http.ResponseWriter, r *http.Request) {
	var req claimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Content == "" {
		writeErr(w, http.StatusBadRequest, "content is required")
		return
	}
	if req.Confidence <= 0 {
		req.Confidence = defaultConfidence(req.Kind)
	}
	frame := req.Frame
	if frame == "" {
		frame = defaultFrame(req.Kind)
	}
	s.ensureFrame(frame)

	now := time.Now()
	id := req.ID
	if id == "" {
		id = newID("self:" + req.Kind)
	} else if !strings.HasPrefix(id, "self:") {
		id = "self:" + id
	}

	// Sentinel record — retracting this cascades suspect marking to the belief.
	sentinelID := "sentinel:" + id
	if err := s.store.Assert(&lumen.Record{
		ID:        sentinelID,
		Content:   "validity sentinel for " + id,
		Timestamp: now,
		Frame:     frame,
	}); err != nil {
		writeErr(w, http.StatusConflict, "sentinel: "+err.Error())
		return
	}

	derivation := append([]string{sentinelID}, req.Sources...)
	if err := s.store.Believe(&lumen.Belief{
		ID:         id,
		Content:    req.Content,
		Confidence: req.Confidence,
		Frame:      frame,
		AssertedAt: now,
		Derivation: derivation,
	}); err != nil {
		// Clean up the sentinel — otherwise it stays active with nothing
		// depending on it, and the next claim with this ID conflicts.
		_ = s.store.Retract(sentinelID, "orphaned: belief assertion failed", now)
		writeErr(w, http.StatusConflict, "believe: "+err.Error())
		return
	}

	s.save()
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":    id,
		"frame": frame,
	})
}

type correctRequest struct {
	ReplacesID string  `json:"replaces_id"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}

func (s *Server) handleSelfCorrect(w http.ResponseWriter, r *http.Request) {
	var req correctRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.ReplacesID == "" || req.Content == "" {
		writeErr(w, http.StatusBadRequest, "replaces_id and content are required")
		return
	}

	now := time.Now()
	reason := req.Reason
	if reason == "" {
		reason = "corrected"
	}

	// Normalise the replaces_id to always carry the self: prefix so callers
	// can use short names (e.g. "my-claim") or fully-qualified names interchangeably.
	replacesID := req.ReplacesID
	if !strings.HasPrefix(replacesID, "self:") {
		replacesID = "self:" + replacesID
	}

	// Retract the prior claim's sentinel, marking it suspect.
	if err := s.store.Retract("sentinel:"+replacesID, reason, now); err != nil {
		writeErr(w, http.StatusNotFound, "prior claim not found: "+err.Error())
		return
	}

	// Assert the replacement.
	conf := req.Confidence
	if conf <= 0 {
		conf = 0.75
	}
	s.ensureFrame("reasoning")
	replacementID := newID("self:corrected")
	sentinelID := "sentinel:" + replacementID
	if err := s.store.Assert(&lumen.Record{
		ID:        sentinelID,
		Content:   "validity sentinel for " + replacementID,
		Timestamp: now,
		Frame:     "reasoning",
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.Believe(&lumen.Belief{
		ID:         replacementID,
		Content:    req.Content,
		Confidence: conf,
		Frame:      "reasoning",
		AssertedAt: now,
		Derivation: []string{sentinelID},
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.save()
	writeJSON(w, http.StatusCreated, map[string]any{
		"retracted_id": req.ReplacesID,
		"new_id":       replacementID,
	})
}

func (s *Server) handleSelfList(w http.ResponseWriter, _ *http.Request) {
	now := time.Now()
	all := s.store.AllBeliefs(now)

	type entry struct {
		ID         string  `json:"id"`
		Content    string  `json:"content"`
		Confidence float64 `json:"confidence"`
		Frame      string  `json:"frame"`
		State      string  `json:"state"`
	}
	var out []entry
	for _, b := range all {
		if !strings.HasPrefix(b.BeliefID, "self:") {
			continue
		}
		out = append(out, entry{
			ID:         b.BeliefID,
			Content:    b.Content,
			Confidence: b.CurrentConfidence,
			Frame:      b.Frame,
			State:      stateStr(b.State),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Confidence > out[j].Confidence
	})
	writeJSON(w, http.StatusOK, out)
}

// handleSelfContext returns a context block containing only self-model claims.
// Designed to be injected as a distinct section, separate from the general belief context.
func (s *Server) handleSelfContext(w http.ResponseWriter, _ *http.Request) {
	now := time.Now()
	all := s.store.AllBeliefs(now)

	type entry struct {
		content    string
		confidence float64
		frame      string
		state      lumen.BeliefState
	}
	var claims []entry
	for _, b := range all {
		if !strings.HasPrefix(b.BeliefID, "self:") {
			continue
		}
		if b.CurrentConfidence < s.cfg.ContextMinConfidence {
			continue
		}
		if b.State == lumen.BeliefSuperseded {
			continue
		}
		claims = append(claims, entry{
			content:    b.Content,
			confidence: b.CurrentConfidence,
			frame:      b.Frame,
			state:      b.State,
		})
	}
	sort.Slice(claims, func(i, j int) bool {
		return claims[i].confidence > claims[j].confidence
	})

	if len(claims) == 0 {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "No active self-model claims.")
		return
	}

	var sb strings.Builder
	sb.WriteString("## My current epistemic commitments\n\n")
	for _, c := range claims {
		pct := int(c.confidence * 100)
		marker := ""
		if c.state == lumen.BeliefSuspect {
			marker = " ⚠"
		}
		sb.WriteString(fmt.Sprintf("- [%s %d%%%s] %s\n", c.frame, pct, marker, c.content))
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, sb.String())
}

func (s *Server) handleSelfBiography(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !strings.HasPrefix(id, "self:") {
		id = "self:" + id
	}
	bio, err := s.store.EpistemicBiography(id, 0.05, time.Now())
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, bio)
}

func (s *Server) handleSelfFrameReport(w http.ResponseWriter, _ *http.Request) {
	// Build a minimal SelfModel view over the server's store for the frame report.
	// This creates a temporary SelfModel that reads from the same underlying store.
	sm := self.NewSelfModel()
	// Replay all self: beliefs into the SelfModel so it can generate the report.
	now := time.Now()
	all := s.store.AllBeliefs(now)
	for _, b := range all {
		if !strings.HasPrefix(b.BeliefID, "self:") {
			continue
		}
		kind := kindFromID(b.BeliefID)
		frame := b.Frame
		if frame == "" {
			frame = "reasoning"
		}
		_ = sm.Assert(&self.Claim{
			ID:         b.BeliefID,
			Kind:       kind,
			Content:    b.Content,
			Confidence: b.CurrentConfidence,
			Frame:      frame,
			AssertedAt: b.AssertedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"report": sm.FrameReport(now),
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func defaultConfidence(kind string) float64 {
	switch kind {
	case "asserted":
		return 0.75
	case "derived":
		return 0.65
	case "retrieved":
		return 0.80
	case "corrected":
		return 0.80
	default:
		return 0.70
	}
}

func defaultFrame(kind string) string {
	switch kind {
	case "asserted", "derived", "corrected":
		return "reasoning"
	case "retrieved":
		return "retrieved"
	default:
		return "reasoning"
	}
}

func kindFromID(id string) self.ClaimKind {
	// IDs are: self:<kind>-<nanos> or self:corrected-<nanos>
	id = strings.TrimPrefix(id, "self:")
	for _, k := range []self.ClaimKind{self.ClaimAsserted, self.ClaimDerived, self.ClaimRetrieved, self.ClaimCorrected} {
		if strings.HasPrefix(id, string(k)) {
			return k
		}
	}
	return self.ClaimAsserted
}
