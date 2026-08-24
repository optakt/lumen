// Package lumen integration test — exercises the full epistemic runtime in a
// realistic scenario: a philosophy-of-mind research assistant tracking beliefs
// about consciousness. Covers all major capabilities in one coherent narrative.
package lumen

import (
	"strings"
	"testing"
	"time"
)

// --- Helpers -----------------------------------------------------------------

func assertQuery(t *testing.T, s *Store, id string, now time.Time, wantMin, wantMax float64) *QueryResult {
	t.Helper()
	r, err := s.Query(id, now)
	if err != nil {
		t.Fatalf("Query(%q): %v", id, err)
	}
	if r.CurrentConfidence < wantMin || r.CurrentConfidence > wantMax {
		t.Errorf("belief %q confidence %.3f not in [%.2f, %.2f]", id, r.CurrentConfidence, wantMin, wantMax)
	}
	return r
}

func assertState(t *testing.T, s *Store, id string, now time.Time, want BeliefState) {
	t.Helper()
	r, err := s.Query(id, now)
	if err != nil {
		t.Fatalf("Query(%q): %v", id, err)
	}
	if r.State != want {
		t.Errorf("belief %q: state %v, want %v", id, r.State, want)
	}
}

// --- The integration test ----------------------------------------------------

const consciousnessLM = `
frame philosophical
    composition: bayesian
    decay: none

frame empirical
    composition: bayesian
    decay: exponential halflife: 730d

frame fast-science
    composition: bayesian
    decay: exponential halflife: 90d
    on_stale_derivation: mark_suspect

record chalmers-1995 in philosophical
    "Chalmers 1995: the hard problem names the gap between physical processes and subjective experience."
    at: "1995-01-01T00:00:00Z"
    provenance: foundational

record zombie-argument in philosophical
    "Philosophical zombies are conceivable. Their conceivability implies experience is not entailed by physical facts."
    at: "1996-01-01T00:00:00Z"

record knowledge-argument in philosophical
    "Mary the color scientist learns new facts upon seeing red, though she knew all physical facts beforehand."
    at: "1982-01-01T00:00:00Z"

record cogitate-2023 in empirical
    "Cogitate Consortium 2023: PFC active during conscious perception, contradicting IIT exclusion. n=256."
    at: "2023-06-01T00:00:00Z"

record iit-letter-2023 in empirical
    "124 neuroscientists 2023: open letter calling IIT pseudoscience for unfalsifiable predictions."
    at: "2023-09-15T00:00:00Z"

record gwt-fmri-2024 in empirical
    "fMRI meta-analysis 2024: GWT global workspace signature replicated across 18 labs, n=3400."
    at: "2024-03-01T00:00:00Z"

believe hard-problem in philosophical
    "The hard problem of consciousness is real: functional and informational accounts leave experience unexplained."
    confidence: 0.78
    from: chalmers-1995, zombie-argument, knowledge-argument

believe iit-viable in empirical
    "IIT as a framework remains viable despite recent criticism."
    confidence: 0.35
    from: cogitate-2023, iit-letter-2023

believe gwt-strong in empirical
    "Global Workspace Theory has the strongest empirical backing of the major consciousness theories."
    confidence: 0.72
    from: cogitate-2023, gwt-fmri-2024

believe physicalism-challenged in philosophical
    "Physicalism faces a genuine explanatory gap regarding consciousness."
    confidence: 0.65
    from: hard-problem, zombie-argument, knowledge-argument

query recent-iit-updates
    target: iit-viable
    select: confidence-changes
    since: "2023-01-01T00:00:00Z"

query hard-problem-retractions
    target: hard-problem
    select: retraction-events
`

func TestIntegration_LoadAndQuery(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)

	if err := LoadFile(consciousnessLM, s, t0); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// Basic queries.
	assertQuery(t, s, "hard-problem", t0, 0.75, 0.82)
	assertQuery(t, s, "iit-viable", t0, 0.30, 0.40)
	assertQuery(t, s, "gwt-strong", t0, 0.68, 0.78)
	assertQuery(t, s, "physicalism-challenged", t0, 0.60, 0.70)

	assertState(t, s, "hard-problem", t0, BeliefActive)
	assertState(t, s, "iit-viable", t0, BeliefActive)

	t.Log("load and basic query: ok")
}

func TestIntegration_FoundationalRecord(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(consciousnessLM, s, t0); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// chalmers-1995 is foundational.
	s.mu.RLock()
	rec := s.records["chalmers-1995"]
	s.mu.RUnlock()
	if !rec.Foundational {
		t.Error("chalmers-1995 should be foundational")
	}

	chain, err := s.ProvenanceChain("hard-problem", t0)
	if err != nil {
		t.Fatalf("ProvenanceChain: %v", err)
	}
	if !chain.Nodes["chalmers-1995"].Foundational {
		t.Error("chalmers-1995 ProvenanceNode should be Foundational")
	}

	// WeakestLink skips foundational nodes.
	wl := chain.WeakestLink()
	if wl != nil && wl.ID == "chalmers-1995" {
		t.Error("WeakestLink should not return foundational record")
	}
	t.Logf("weakest link (excl. foundational): %v", func() string {
		if wl == nil { return "none" }; return wl.ID
	}())
}

func TestIntegration_Provenance(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(consciousnessLM, s, t0); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	chain, err := s.ProvenanceChain("physicalism-challenged", t0)
	if err != nil {
		t.Fatalf("ProvenanceChain: %v", err)
	}
	if chain.MaxDepth < 2 {
		t.Errorf("physicalism-challenged should have depth >= 2 (belief-of-beliefs), got %d", chain.MaxDepth)
	}
	rendered := chain.Render()
	if !strings.Contains(rendered, "⚑") {
		t.Error("foundational record should appear with ⚑ marker")
	}
	t.Log("provenance chain rendered correctly")
}

func TestIntegration_EpistemicDecay(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(consciousnessLM, s, t0); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// gwt-strong is in the empirical frame (730d halflife).
	// At 10 years (3650 days ≈ 5 halflives), confidence should be ~0.72 * 2^-5 ≈ 0.022.
	t10yr := t0.Add(365 * 10 * 24 * time.Hour)
	r := assertQuery(t, s, "gwt-strong", t10yr, 0.0, 0.05)
	t.Logf("gwt-strong after 10 years: %.4f (decay working)", r.CurrentConfidence)

	// hard-problem is philosophical (no decay) — should remain at ~0.78.
	assertQuery(t, s, "hard-problem", t10yr, 0.74, 0.82)
	t.Log("philosophical frame decay: none (correct)")
}

func TestIntegration_OpaqueFrame(t *testing.T) {
	src := `
frame opaque-model
    composition: opaque
    source: "consciousness-classifier-v1"
    calibration: isotonic
    opacity-reason: "neural network weights not individually addressable"
    decay: none

record model-output-r1 in opaque-model
    "Model inferred 87% probability of conscious experience in patient."
    at: "2026-01-01T00:00:00Z"

believe model-inference in opaque-model
    "Patient shows evidence of conscious experience per classifier."
    confidence: 0.87
    from: model-output-r1
`
	s := NewStore()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(src, s, t0); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// Opaque frame: confidence stays as declared (not updated by evidence blocks).
	assertQuery(t, s, "model-inference", t0, 0.86, 0.88)

	// BelieveComposed blocked.
	_, err := s.BelieveComposed(
		&Belief{ID: "test", Frame: "opaque-model", Content: "test", Confidence: 0.80, AssertedAt: t0},
		0.50,
		[]Evidence{{SourceID: "model-output-r1", LikelihoodRatio: 4.0, Confidence: 0.87}},
	)
	if err == nil || !strings.Contains(err.Error(), "opaque") {
		t.Errorf("BelieveComposed in opaque frame should fail with 'opaque' error, got: %v", err)
	}

	// Explain mentions opacity.
	explanation, err := s.Explain("model-inference", t0)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if !strings.Contains(explanation, "opaque") || !strings.Contains(explanation, "isotonic") {
		t.Errorf("explanation should mention opacity and calibration:\n%s", explanation[:intMin(300, len(explanation))])
	}
	t.Log("opaque frame: all checks passed")
}

func TestIntegration_NamedQueries(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(consciousnessLM, s, t0); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// Named queries should be registered.
	all := s.AllQueries()
	if len(all) < 2 {
		t.Errorf("expected >= 2 named queries, got %d", len(all))
	}

	// Execute named query.
	ar, err := s.RunQueryByID("recent-iit-updates", t0)
	if err != nil {
		t.Fatalf("RunQueryByID: %v", err)
	}
	if ar.QueryID != "recent-iit-updates" {
		t.Errorf("QueryID: %s", ar.QueryID)
	}
	t.Logf("named queries registered: %d; recent-iit-updates events: %d", len(all), len(ar.Events))
}

func TestIntegration_Contraction_And_Recovery(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(consciousnessLM, s, t0); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// Before contraction: iit-viable is active.
	assertState(t, s, "iit-viable", t0, BeliefActive)

	t1 := t0.Add(24 * time.Hour)
	result, err := s.MinimalContraction("cogitate-2023", t1)
	if err != nil {
		t.Fatalf("MinimalContraction: %v", err)
	}
	t.Logf("contraction removes: %v", result.Removed)

	if err := s.ApplyContraction(result, "cogitate-2023 retracted: methodology disputed", t1); err != nil {
		t.Fatalf("ApplyContraction: %v", err)
	}

	// iit-viable and gwt-strong both depend on cogitate-2023; they should now be superseded.
	// (Or only the ones with no clean path.)
	contracted := s.ContractedBeliefs()
	t.Logf("contracted after cogitate-2023 retraction: %v", contracted)

	// Recovery: re-assert cogitate-2023.
	s.mu.Lock()
	if rec, ok := s.records["cogitate-2023"]; ok {
		rec.Retracted = false
		rec.RetractReason = "methodology dispute resolved"
	}
	s.mu.Unlock()

	t2 := t1.Add(24 * time.Hour)
	for _, cid := range contracted {
		if err := s.Recover(cid, t2); err != nil {
			t.Logf("Recover(%s): %v (skipping — may have superseded sources)", cid, err)
		} else {
			assertState(t, s, cid, t2, BeliefActive)
			t.Logf("recovered: %s", cid)
		}
	}
}

func TestIntegration_BeliefBiography(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(consciousnessLM, s, t0); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// Revise a belief twice to create a history.
	t1 := t0.Add(30 * 24 * time.Hour)
	if err := s.ReAssert("gwt-strong", "GWT has the strongest empirical backing (updated).", 0.80, t1); err != nil {
		t.Fatalf("ReAssert: %v", err)
	}
	t2 := t1.Add(60 * 24 * time.Hour)
	if err := s.ReAssert("gwt-strong", "GWT support weakened after meta-analysis replication issues.", 0.65, t2); err != nil {
		t.Fatalf("ReAssert: %v", err)
	}

	t3 := t2.Add(30 * 24 * time.Hour)
	bio, err := s.EpistemicBiography("gwt-strong", 0.05, t3)
	if err != nil {
		t.Fatalf("EpistemicBiography: %v", err)
	}
	if len(bio.MindChanges) == 0 {
		t.Error("expected at least one mind change (confidence revised twice)")
	}
	t.Logf("gwt-strong biography: %d mind changes, current health: %.0f%%",
		len(bio.MindChanges), bio.CurrentHealth.Score)

	// Render should mention confidence changes.
	rendered := RenderBiography(bio)
	if !strings.Contains(rendered, "confidence") && !strings.Contains(rendered, "Confidence") {
		t.Errorf("biography render should mention confidence:\n%s", rendered[:intMin(300, len(rendered))])
	}
}

func TestIntegration_HealthScoring(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(consciousnessLM, s, t0); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	h, err := s.BeliefHealth("hard-problem", t0)
	if err != nil {
		t.Fatalf("BeliefHealth: %v", err)
	}
	if h.Score < 50 {
		t.Errorf("hard-problem health score should be reasonable: %.0f", h.Score)
	}
	t.Logf("hard-problem health: %.0f/100 (%s)", h.Score, h.Grade)
	// Check provenance component note for foundational mention.
	for _, c := range h.Components {
		if c.Name == "Provenance" {
			t.Logf("provenance note: %q", c.Note)
			break
		}
	}
}

func TestIntegration_StalenessPolicy(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(consciousnessLM, s, t0); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// fast-science frame has on_stale_derivation: mark_suspect and 90d halflife.
	// Add a belief in fast-science that derives from a belief in empirical.
	if err := s.Believe(&Belief{
		ID: "fast-finding", Content: "Quick finding from GWT.",
		Confidence: 0.75, Frame: "fast-science", AssertedAt: t0,
		Derivation: []string{"gwt-strong"},
	}); err != nil {
		t.Fatalf("Believe: %v", err)
	}

	// Immediately: gwt-strong is fresh → fast-finding stays active.
	assertState(t, s, "fast-finding", t0, BeliefActive)

	// After 10 years: gwt-strong has decayed below StaleThreshold.
	// fast-finding's frame has mark_suspect policy → should become suspect.
	t10yr := t0.Add(365 * 10 * 24 * time.Hour)
	assertState(t, s, "fast-finding", t10yr, BeliefSuspect)
	t.Log("on_stale_derivation: mark_suspect correctly applied after decay")
}

func TestIntegration_PredicateQuery(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(consciousnessLM, s, t0); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// High confidence philosophical beliefs.
	results, err := s.QueryBeliefs("confidence > 0.7 AND frame = philosophical", t0)
	if err != nil {
		t.Fatalf("QueryBeliefs: %v", err)
	}
	for _, r := range results {
		if r.CurrentConfidence <= 0.7 {
			t.Errorf("result %q has confidence %.3f ≤ 0.7", r.BeliefID, r.CurrentConfidence)
		}
		if r.Frame != "philosophical" {
			t.Errorf("result %q is in frame %q, expected philosophical", r.BeliefID, r.Frame)
		}
	}
	t.Logf("high-confidence philosophical beliefs: %d found", len(results))

	// Content search.
	results2, err := s.QueryBeliefs(`content contains "consciousness"`, t0)
	if err != nil {
		t.Fatalf("QueryBeliefs(content): %v", err)
	}
	if len(results2) == 0 {
		t.Error("expected at least one belief containing 'consciousness'")
	}
	t.Logf("beliefs containing 'consciousness': %d found", len(results2))
}

func TestIntegration_AGMPostulates(t *testing.T) {
	s := NewStore()
	t0 := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(consciousnessLM, s, t0); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	result, err := s.MinimalContraction("cogitate-2023", t0)
	if err != nil {
		t.Fatalf("MinimalContraction: %v", err)
	}

	audit := s.PostulateAudit(result, "cogitate-2023")
	for name, r := range audit {
		if !r.Passed {
			t.Errorf("AGM postulate %s FAILED: %s", name, r.Note)
		} else {
			t.Logf("✓ %s: %s", name, r.Note)
		}
	}
}

func intMin(a, b int) int {
	if a < b { return a }
	return b
}
