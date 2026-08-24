package server

import (
	"encoding/json"
	"net/http"
	"time"

	lumen "github.com/optakt/lumen"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ─── Records ──────────────────────────────────────────────────────────────────

type recordRequest struct {
	ID        string `json:"id,omitempty"`
	Content   string `json:"content"`
	Frame     string `json:"frame,omitempty"`
}

func (s *Server) handleAssertRecord(w http.ResponseWriter, r *http.Request) {
	var req recordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Content == "" {
		writeErr(w, http.StatusBadRequest, "content is required")
		return
	}
	frame := req.Frame
	if frame == "" {
		frame = s.cfg.DefaultFrame
	}
	s.ensureFrame(frame)

	id := req.ID
	if id == "" {
		id = newID("rec")
	}
	if err := s.store.Assert(&lumen.Record{
		ID:        id,
		Content:   req.Content,
		Frame:     frame,
		Timestamp: time.Now(),
	}); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	s.save()
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// ─── Beliefs ──────────────────────────────────────────────────────────────────

type beliefRequest struct {
	ID         string   `json:"id,omitempty"`
	Content    string   `json:"content"`
	Confidence float64  `json:"confidence"`
	Frame      string   `json:"frame,omitempty"`
	Sources    []string `json:"sources,omitempty"`
}

func (s *Server) handleBelieve(w http.ResponseWriter, r *http.Request) {
	var req beliefRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Content == "" {
		writeErr(w, http.StatusBadRequest, "content is required")
		return
	}
	if req.Confidence > 1 {
		writeErr(w, http.StatusBadRequest, "confidence must be in (0, 1]")
		return
	}
	if req.Confidence <= 0 {
		req.Confidence = 0.7 // absent from request — default, not a rewrite
	}
	frame := req.Frame
	if frame == "" {
		frame = s.cfg.DefaultFrame
	}
	s.ensureFrame(frame)

	id := req.ID
	if id == "" {
		id = newID("bel")
	}
	if err := s.store.Believe(&lumen.Belief{
		ID:         id,
		Content:    req.Content,
		Confidence: req.Confidence,
		Frame:      frame,
		AssertedAt: time.Now(),
		Derivation: req.Sources,
	}); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	s.save()
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// ─── List ─────────────────────────────────────────────────────────────────────

type beliefSummary struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
	Frame      string  `json:"frame"`
	State      string  `json:"state"`
}

func (s *Server) handleListBeliefs(w http.ResponseWriter, _ *http.Request) {
	results := s.store.AllBeliefs(time.Now())
	out := make([]beliefSummary, 0, len(results))
	for _, b := range results {
		out = append(out, beliefSummary{
			ID:         b.BeliefID,
			Content:    b.Content,
			Confidence: b.CurrentConfidence,
			Frame:      b.Frame,
			State:      stateStr(b.State),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// ─── Retract ──────────────────────────────────────────────────────────────────

type retractRequest struct {
	RecordID string `json:"record_id"`
	Reason   string `json:"reason,omitempty"`
}

func (s *Server) handleRetract(w http.ResponseWriter, r *http.Request) {
	var req retractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.RecordID == "" {
		writeErr(w, http.StatusBadRequest, "record_id is required")
		return
	}
	if err := s.store.Retract(req.RecordID, req.Reason, time.Now()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.save()
	w.WriteHeader(http.StatusNoContent)
}

// ─── Explain ──────────────────────────────────────────────────────────────────

func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	explanation, err := s.store.Explain(id, time.Now())
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"explanation": explanation})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (s *Server) ensureFrame(name string) {
	s.store.RegisterFrame(lumen.Frame{
		Name:        name,
		Composition: lumen.CompositionBayesian,
		Decay: lumen.DecayPolicy{
			Kind:     lumen.DecayExponential,
			Halflife: 7 * 24 * time.Hour,
		},
	})
}

func stateStr(st lumen.BeliefState) string {
	switch st {
	case lumen.BeliefActive:
		return "active"
	case lumen.BeliefSuspect:
		return "suspect"
	case lumen.BeliefStale:
		return "stale"
	case lumen.BeliefSuperseded:
		return "superseded"
	default:
		return "unknown"
	}
}
