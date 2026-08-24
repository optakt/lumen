package lumen

import (
	"os"
	"testing"
	"time"
)

func TestNewSyntaxParse(t *testing.T) {
	src, err := os.ReadFile("testdata/new-syntax.lm")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	result, err := ParseFull(string(src))
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}

	// Frames
	framesByName := make(map[string]ParsedFrame)
	for _, f := range result.Frames {
		framesByName[f.Name] = f
	}

	// Opaque frame
	nd := framesByName["neural-diagnostic"]
	if !nd.Opaque {
		t.Error("neural-diagnostic should be opaque")
	}
	if nd.OpaqueSource != "cardiovascular_v3" {
		t.Errorf("opaque source: got %q, want cardiovascular_v3", nd.OpaqueSource)
	}
	if nd.Calibration != "isotonic" {
		t.Errorf("calibration: got %q, want isotonic", nd.Calibration)
	}
	t.Logf("opaque frame: source=%s calibration=%s reason=%q", nd.OpaqueSource, nd.Calibration, nd.OpaqueReason)

	// Foundational record
	var foundational *ParsedRecord
	for i := range result.Records {
		if result.Records[i].ID == "prior-assumption" {
			foundational = &result.Records[i]
		}
	}
	if foundational == nil {
		t.Error("prior-assumption record not found")
	} else if !foundational.Foundational {
		t.Error("prior-assumption should be foundational")
	} else {
		t.Logf("foundational record: %s", foundational.ID)
	}

	// Date-only timestamp
	var cantor *ParsedRecord
	for i := range result.Records {
		if result.Records[i].ID == "cantor-1891" {
			cantor = &result.Records[i]
		}
	}
	if cantor == nil {
		t.Error("cantor-1891 record not found")
	} else if cantor.At == nil {
		t.Error("cantor-1891 should have a timestamp from date-only format")
	} else {
		t.Logf("date-only timestamp: %v", cantor.At)
	}

	// Belief with inline evidence
	var hpBelief *ParsedBelief
	for i := range result.Beliefs {
		if result.Beliefs[i].ID == "hard-problem-real" {
			hpBelief = &result.Beliefs[i]
		}
	}
	if hpBelief == nil {
		t.Fatal("hard-problem-real belief not found")
	}
	if !hpBelief.HasCredalPrior {
		t.Error("hard-problem-real should have credal prior")
	} else {
		t.Logf("credal prior: [%.2f, %.2f]", hpBelief.CredalPriorLo, hpBelief.CredalPriorHi)
	}
	if len(hpBelief.Evidence) != 2 {
		t.Errorf("expected 2 evidence blocks, got %d", len(hpBelief.Evidence))
	} else {
		for _, ev := range hpBelief.Evidence {
			t.Logf("evidence %s: lr=[%.1f, %.1f] conf=%.2f source=%s corr=%v",
				ev.ID, ev.LRLo, ev.LRHi, ev.Confidence, ev.Source, ev.CorrelatesWith)
		}
		// Check correlation declared
		knowledgeEv := hpBelief.Evidence[1]
		if r, ok := knowledgeEv.CorrelatesWith["zombie-ev"]; !ok || r < 0.5 {
			t.Errorf("knowledge-ev should correlate with zombie-ev ~0.55, got %v", knowledgeEv.CorrelatesWith)
		}
	}

	// Bridge
	if len(result.Bridges) == 0 {
		t.Error("expected bridge declaration")
	} else {
		br := result.Bridges[0]
		t.Logf("bridge: %s from=%s to=%s verified=%v", br.Name, br.FromFrame, br.ToFrame, br.Verified)
	}

	// Query
	if len(result.Queries) == 0 {
		t.Error("expected query declaration")
	} else {
		q := result.Queries[0]
		t.Logf("query: %s target=%s select=%s since=%s", q.ID, q.Target, q.Select, q.Since)
		if q.Target != "hard-problem-real" {
			t.Errorf("query target: got %q, want hard-problem-real", q.Target)
		}
	}
}

func TestNewSyntaxLoadFile(t *testing.T) {
	src, err := os.ReadFile("testdata/new-syntax.lm")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	s := NewStore()
	now := time.Now()
	if err := LoadFile(string(src), s, now); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// Verify credal update ran: confidence should be higher than prior midpoint
	// prior: [0.35, 0.65] → midpoint 0.50
	// With LR=[3,6] and LR=[2.5,5] evidence, posterior midpoint should be > 0.50
	r, err := s.Query("hard-problem-real", now)
	if err != nil {
		t.Fatalf("Query hard-problem-real: %v", err)
	}
	t.Logf("hard-problem-real confidence after evidence: %.4f", r.CurrentConfidence)
	if r.CurrentConfidence <= 0.50 {
		t.Errorf("expected confidence > 0.50 after positive evidence, got %.4f", r.CurrentConfidence)
	}

	// Foundational record exists with marker
	s.mu.RLock()
	pa, ok := s.records["prior-assumption"]
	s.mu.RUnlock()
	if !ok {
		t.Error("prior-assumption record not in store")
	} else {
		t.Logf("foundational record content: %s", pa.Content[:40])
	}

	// Date-only timestamp loaded
	s.mu.RLock()
	cantor, ok := s.records["cantor-1891"]
	s.mu.RUnlock()
	if !ok {
		t.Error("cantor-1891 not in store")
	} else if cantor.Timestamp.Year() != 1891 {
		t.Errorf("cantor-1891 timestamp year: got %d, want 1891", cantor.Timestamp.Year())
	} else {
		t.Logf("cantor-1891 timestamp: %v", cantor.Timestamp)
	}

	// Bridge registered
	br, _ := s.Bridges.Lookup("philosophical-to-empirical")
	if br == nil {
		t.Error("bridge philosophical-to-empirical not registered")
	} else {
		t.Logf("bridge: from=%s to=%s verified=%v", br.FromFrame, br.ToFrame, br.Verified)
	}
}
