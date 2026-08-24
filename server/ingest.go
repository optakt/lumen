package server

import (
	"encoding/json"
	"net/http"
	"time"

	lumen "github.com/optakt/lumen"
)

type ingestRequest struct {
	Text          string   `json:"text"`
	Frame         string   `json:"frame,omitempty"`
	MinConfidence *float64 `json:"min_confidence,omitempty"`
}

type ingestResponse struct {
	AssertedBeliefs []string `json:"asserted_beliefs"`
	AssertedRecords []string `json:"asserted_records"`
	Skipped         int      `json:"skipped"`
}

// handleIngest extracts belief candidates from raw text and asserts them into the store.
func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	var req ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Text == "" {
		writeErr(w, http.StatusBadRequest, "text is required")
		return
	}

	minConf := s.cfg.IngestMinConfidence
	if req.MinConfidence != nil {
		minConf = *req.MinConfidence
	}

	now := time.Now()
	analysis := lumen.AnalyzeText(req.Text)

	// Use caller-specified frame, then NLP suggestion, then default.
	frame := req.Frame
	if frame == "" && analysis.Frame != "" {
		frame = analysis.Frame
	}
	if frame == "" {
		frame = s.cfg.DefaultFrame
	}
	s.ensureFrame(frame)

	resp := ingestResponse{}

	// Assert records first (they become sources for derived beliefs).
	recordIDs := make(map[string]string) // extracted ID → store ID
	for _, rec := range analysis.Records {
		if rec.Confidence < minConf {
			resp.Skipped++
			continue
		}
		id := newID("ing-rec")
		r := &lumen.Record{
			ID:        id,
			Content:   rec.Content,
			Frame:     frame,
			Timestamp: now,
		}
		if err := s.store.Assert(r); err != nil {
			resp.Skipped++
			continue
		}
		recordIDs[rec.ID] = id
		resp.AssertedRecords = append(resp.AssertedRecords, id)
	}

	// Assert beliefs, linking to any records just asserted.
	for _, bel := range analysis.Beliefs {
		if bel.Confidence < minConf {
			resp.Skipped++
			continue
		}
		var sources []string
		for _, srcID := range bel.DerivedFrom {
			if storeID, ok := recordIDs[srcID]; ok {
				sources = append(sources, storeID)
			}
		}
		id := newID("ing-bel")
		b := &lumen.Belief{
			ID:         id,
			Content:    bel.Content,
			Confidence: bel.Confidence,
			Frame:      frame,
			AssertedAt: now,
			Derivation: sources,
		}
		if err := s.store.Believe(b); err != nil {
			resp.Skipped++
			continue
		}
		resp.AssertedBeliefs = append(resp.AssertedBeliefs, id)
	}

	s.save()
	writeJSON(w, http.StatusOK, resp)
}
