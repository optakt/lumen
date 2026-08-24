package lumen

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExportJSON(t *testing.T) {
	s, now := setupSearchStore(t)
	data, err := s.ExportJSON(now)
	if err != nil { t.Fatalf("ExportJSON: %v", err) }

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Invalid JSON output: %v", err)
	}
	if _, ok := parsed["beliefs"]; !ok { t.Error("JSON should contain 'beliefs'") }
	if _, ok := parsed["records"]; !ok { t.Error("JSON should contain 'records'") }
	t.Logf("JSON export: %d bytes, %d beliefs", len(data), int(parsed["beliefs"].([]interface{})[0].(map[string]interface{})["confidence"].(float64)*100))
}

func TestExportMarkdown(t *testing.T) {
	s, now := setupSearchStore(t)
	md := s.ExportMarkdown("Test Knowledge Base", now)
	if !strings.Contains(md, "# Test Knowledge Base") { t.Error("title missing") }
	if !strings.Contains(md, "**") { t.Error("bold confidence values missing") }
	if !strings.Contains(md, "Records") { t.Error("records section missing") }
	t.Logf("Markdown:\n%s", md)
}

func TestExportLM(t *testing.T) {
	s, now := setupSearchStore(t)
	lm := s.ExportLM(now)
	if !strings.Contains(lm, "record ") { t.Error("records missing") }
	if !strings.Contains(lm, "believe ") { t.Error("beliefs missing") }
	if !strings.Contains(lm, "confidence:") { t.Error("confidence missing") }
	t.Logf("LM export: %d bytes", len(lm))
}

func TestExportVersionHistory(t *testing.T) {
	s, now := setupSearchStore(t)

	// Create some version history
	s.Revise("r1", "Updated Cogitate finding", 0, "new analysis", now.Add(3600))
	s.ReAssert("iit-weakened", "IIT is strongly weakened by updated Cogitate findings.", 0.75, now.Add(7200))

	h, err := s.BeliefHistory("iit-weakened")
	if err != nil { t.Fatalf("BeliefHistory: %v", err) }
	t.Logf("Version history for iit-weakened (%d versions):", len(h))
	t.Log(RenderHistory("iit-weakened", h))

	// Export and verify versions appear in JSON
	data, _ := s.ExportJSON(now.Add(7200))
	if !strings.Contains(string(data), "iit-weakened") {
		t.Error("exported belief should include iit-weakened")
	}
}

func TestExportLMRoundtrip(t *testing.T) {
	// Build a store with frames, records (including foundational), and beliefs.
	// Export to .lm, re-parse, verify the same beliefs are present.
	s := NewStore()
	s.RegisterFrame(Frame{
		Name: "philo", Decay: DecayPolicy{Kind: "none"},
	})
	s.RegisterFrame(Frame{
		Name: "empirical",
		Decay: DecayPolicy{Kind: "exponential", Halflife: 365 * 24 * time.Hour},
		OnStaleDerivation: "mark_suspect",
	})

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = s.Assert(&Record{ID: "r-axiom", Content: "First principles.", Frame: "philo", Timestamp: now, Foundational: true})
	_ = s.Assert(&Record{ID: "r-data", Content: "Empirical observation.", Frame: "empirical", Timestamp: now})
	_ = s.Believe(&Belief{ID: "b1", Content: "Core belief.", Confidence: 0.80, Frame: "philo", AssertedAt: now, Derivation: []string{"r-axiom"}})
	_ = s.Believe(&Belief{ID: "b2", Content: "Empirical belief.", Confidence: 0.65, Frame: "empirical", AssertedAt: now, Derivation: []string{"r-data", "b1"}})

	exported := s.ExportLM(now)
	t.Logf("exported .lm:\n%s", exported)

	// Parse the exported .lm.
	s2 := NewStore()
	if err := LoadFile(exported, s2, now); err != nil {
		t.Fatalf("LoadFile on exported .lm: %v", err)
	}

	// Verify frames.
	s2.mu.RLock()
	_, hasPhilo := s2.frames["philo"]
	_, hasEmpirical := s2.frames["empirical"]
	s2.mu.RUnlock()
	if !hasPhilo || !hasEmpirical {
		t.Errorf("frames missing after round-trip: philo=%v empirical=%v", hasPhilo, hasEmpirical)
	}

	// Verify records.
	s2.mu.RLock()
	axiom, hasAxiom := s2.records["r-axiom"]
	_, hasData := s2.records["r-data"]
	s2.mu.RUnlock()
	if !hasAxiom || !hasData {
		t.Errorf("records missing after round-trip")
	}
	if axiom != nil && !axiom.Foundational {
		t.Error("r-axiom should be foundational after round-trip")
	}

	// Verify beliefs.
	r1, err := s2.Query("b1", now)
	if err != nil {
		t.Fatalf("b1 not found after round-trip: %v", err)
	}
	if r1.CurrentConfidence < 0.78 || r1.CurrentConfidence > 0.82 {
		t.Errorf("b1 confidence %.3f not in [0.78, 0.82]", r1.CurrentConfidence)
	}
	r2, err := s2.Query("b2", now)
	if err != nil {
		t.Fatalf("b2 not found after round-trip: %v", err)
	}
	if r2.CurrentConfidence < 0.63 || r2.CurrentConfidence > 0.67 {
		t.Errorf("b2 confidence %.3f not in [0.63, 0.67]", r2.CurrentConfidence)
	}

	// Verify on_stale_derivation survived.
	s2.mu.RLock()
	ef := s2.frames["empirical"]
	s2.mu.RUnlock()
	if ef.OnStaleDerivation != "mark_suspect" {
		t.Errorf("OnStaleDerivation: got %q, want mark_suspect", ef.OnStaleDerivation)
	}
	// Verify named query survived export/import.
	// First register a query in the original store and re-export.
	s.AddQuery(ParsedQuery{ID: "test-query", Target: "b1", Select: "confidence-changes", Since: "2026-01-01T00:00:00Z"})
	exported2 := s.ExportLM(now)
	s3 := NewStore()
	if err := LoadFile(exported2, s3, now); err != nil {
		t.Fatalf("LoadFile with queries: %v", err)
	}
	q, ok := s3.GetQuery("test-query")
	if !ok {
		t.Error("named query should survive ExportLM/LoadFile round-trip")
	} else if q.Target != "b1" || q.Select != "confidence-changes" {
		t.Errorf("query round-trip: target=%q select=%q", q.Target, q.Select)
	}
	t.Log("ExportLM round-trip: all checks passed")
}
