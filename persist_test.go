package lumen

import (
	"os"
	"testing"
	"time"
)

func TestSaveAndLoadStore(t *testing.T) {
	// Build a store with some state
	s := NewStore()
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	s.RegisterFrame(Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayExponential, Halflife: 5 * 365 * 24 * time.Hour}})
	s.RegisterFrame(Frame{Name: "philosophical", Decay: DecayPolicy{Kind: DecayNone}})

	s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "The Cogitate study found IIT predictions unconfirmed.", Timestamp: now.AddDate(-1, 0, 0)})
	s.Assert(&Record{ID: "r2", Frame: "philosophical", Content: "Chalmers introduced the hard problem in 1995.", Timestamp: now.AddDate(-2, 0, 0)})
	s.Believe(&Belief{ID: "b1", Frame: "empirical", Content: "IIT is significantly weakened.", Confidence: 0.70, AssertedAt: now, Derivation: []string{"r1"}})
	s.Believe(&Belief{ID: "b2", Frame: "philosophical", Content: "The hard problem remains unsolved.", Confidence: 0.72, AssertedAt: now, Derivation: []string{"r2"}})

	// Save
	dbPath := t.TempDir() + "/test.db"
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := SaveStore(s, db); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	db.Close()

	// Load fresh
	db2, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB(2): %v", err)
	}
	defer db2.Close()

	s2, err := LoadStore(db2, now)
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}

	// Verify records
	s2.mu.RLock()
	if len(s2.records) != 2 {
		t.Errorf("expected 2 records, got %d", len(s2.records))
	}
	if len(s2.beliefs) != 2 {
		t.Errorf("expected 2 beliefs, got %d", len(s2.beliefs))
	}
	s2.mu.RUnlock()

	// Query a belief
	q, err := s2.Query("b1", now)
	if err != nil {
		t.Fatalf("Query b1: %v", err)
	}
	if q.Content != "IIT is significantly weakened." {
		t.Errorf("unexpected content: %q", q.Content)
	}
	t.Logf("b1 after reload: confidence=%.2f content=%q", q.CurrentConfidence, q.Content)

	// Verify derivation graph was restored
	downstream := s2.Graph.ReachableByDerivation("r1")
	if len(downstream) == 0 {
		t.Error("derivation graph not restored: r1 should have downstream beliefs")
	}
	t.Logf("r1 downstream after reload: %v", downstream)

	// Verify temporal graph was restored
	timeline := s2.Temporal.Timeline()
	if len(timeline) == 0 {
		t.Error("temporal graph not restored")
	}
	t.Logf("Timeline after reload: %d events", len(timeline))

	// File should exist on disk
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("DB file not found after save: %v", err)
	}
}

func TestRoundtripRetractedRecord(t *testing.T) {
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "test", Decay: DecayPolicy{Kind: DecayNone}})
	s.Assert(&Record{ID: "r1", Frame: "test", Content: "Flawed finding", Timestamp: now})
	s.Believe(&Belief{ID: "b1", Frame: "test", Content: "Conclusion", Confidence: 0.8, AssertedAt: now, Derivation: []string{"r1"}})
	s.Retract("r1", "methodology error", now)

	dbPath := t.TempDir() + "/retract.db"
	db, _ := OpenDB(dbPath)
	SaveStore(s, db)
	db.Close()

	db2, _ := OpenDB(dbPath)
	defer db2.Close()
	s2, err := LoadStore(db2, now)
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}

	s2.mu.RLock()
	r := s2.records["r1"]
	s2.mu.RUnlock()
	if r == nil || !r.Retracted {
		t.Error("retracted record not preserved through roundtrip")
	}
	t.Logf("r1 retracted=%v reason=%q", r.Retracted, r.RetractReason)

	// b1 should still be suspect
	q, _ := s2.Query("b1", now)
	if q.State != BeliefSuspect {
		t.Errorf("expected b1 to be suspect after reload, got %v", q.State)
	}
}

func TestPersistNewFields(t *testing.T) {
	// Verify that fields added since the initial persist.go was written
	// survive a BoltDB save/load round-trip:
	//   Frame.OnStaleDerivation, Record.Foundational, Belief.ContractedBy.
	dbPath := t.TempDir() + "/persist_new.db"
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}

	s := NewStore()

	now := time.Now()

	// Frame with OnStaleDerivation set.
	s.RegisterFrame(Frame{
		Name:              "monitored",
		Decay:             DecayPolicy{Kind: DecayExponential, Halflife: 90 * 24 * time.Hour},
		OnStaleDerivation: StaleMarkSuspect,
	})

	// Foundational record.
	if err := s.Assert(&Record{
		ID:           "axiom-persist",
		Content:      "Foundational axiom.",
		Frame:        "monitored",
		Timestamp:    now,
		Foundational: true,
	}); err != nil {
		t.Fatalf("Assert foundational: %v", err)
	}

	// Contracted belief.
	if err := s.Believe(&Belief{
		ID:           "belief-contracted",
		Content:      "This belief was contracted.",
		Confidence:   0.80,
		Frame:        "monitored",
		AssertedAt:   now,
		Derivation:   []string{"axiom-persist"},
		State:        BeliefSuperseded,
		ContractedBy: "axiom-persist",
	}); err != nil {
		t.Fatalf("Believe contracted: %v", err)
	}

	// Save.
	if err := SaveStore(s, db); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	db.Close()

	// Load into fresh store.
	db2, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB(2): %v", err)
	}
	defer db2.Close()
	s2, err := LoadStore(db2, now)
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}

	// Verify Frame.OnStaleDerivation.
	s2.mu.RLock()
	frame := s2.frames["monitored"]
	s2.mu.RUnlock()
	if frame.OnStaleDerivation != StaleMarkSuspect {
		t.Errorf("OnStaleDerivation: got %q, want mark_suspect", frame.OnStaleDerivation)
	}

	// Verify Record.Foundational.
	s2.mu.RLock()
	rec := s2.records["axiom-persist"]
	s2.mu.RUnlock()
	if !rec.Foundational {
		t.Error("Foundational should be true after round-trip")
	}

	// Verify Belief.ContractedBy.
	s2.mu.RLock()
	b := s2.beliefs["belief-contracted"]
	s2.mu.RUnlock()
	if b.ContractedBy != "axiom-persist" {
		t.Errorf("ContractedBy: got %q, want axiom-persist", b.ContractedBy)
	}
	if b.State != BeliefSuperseded {
		t.Errorf("State: got %v, want BeliefSuperseded", b.State)
	}

	t.Logf("all new fields survive BoltDB round-trip: OnStaleDerivation=%q Foundational=%v ContractedBy=%q",
		frame.OnStaleDerivation, rec.Foundational, b.ContractedBy)
}

// TestPersistCompositionFields verifies that BelieveComposed metadata
// (CompositionPrior, CompositionEvidence) survives a BoltDB round-trip.
// Without this, FragilityScan loses the exact sensitivity path after restart.
func TestPersistCompositionFields(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "reasoning", Decay: DecayPolicy{Kind: DecayNone}})

	if err := s.Assert(&Record{ID: "r-src", Frame: "reasoning", Content: "source record", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	evidence := []Evidence{{SourceID: "r-src", Confidence: 0.9, LikelihoodRatio: 4.0}}
	b := &Belief{
		ID: "b-comp", Frame: "reasoning", Content: "composed claim",
		Confidence: 0.8, AssertedAt: now, Derivation: []string{"r-src"},
	}
	if _, err := s.BelieveComposed(b, 0.5, evidence); err != nil {
		t.Fatal(err)
	}

	if err := SaveStore(s, db); err != nil {
		t.Fatal(err)
	}

	s2, err := LoadStore(db, now)
	if err != nil {
		t.Fatal(err)
	}

	s2.mu.RLock()
	loaded := s2.beliefs["b-comp"]
	s2.mu.RUnlock()
	if loaded == nil {
		t.Fatal("composed belief lost after BoltDB round-trip")
	}
	if loaded.CompositionPrior != 0.5 {
		t.Errorf("CompositionPrior: want 0.5, got %g", loaded.CompositionPrior)
	}
	if len(loaded.CompositionEvidence) != 1 {
		t.Fatalf("CompositionEvidence: want 1 block, got %d", len(loaded.CompositionEvidence))
	}
	if loaded.CompositionEvidence[0].SourceID != "r-src" {
		t.Errorf("evidence source: want r-src, got %s", loaded.CompositionEvidence[0].SourceID)
	}
	if loaded.CompositionEvidence[0].LikelihoodRatio != 4.0 {
		t.Errorf("LR: want 4.0, got %g", loaded.CompositionEvidence[0].LikelihoodRatio)
	}
	// FragilityScan must use the exact path after reload.
	entries := s2.FragilityScan(now)
	found := false
	for _, e := range entries {
		if e.BeliefID == "b-comp" && e.WeakestKind == "evidence" {
			found = true
		}
	}
	if !found {
		t.Error("FragilityScan did not use exact sensitivity path after BoltDB reload")
	}
}

// TestPersistCrossFrameFields verifies CrossFrame snapshots survive BoltDB.
// Without this, the retrodiction fix is undone after a restart: reloaded
// beliefs would fall back to the legacy ImportedDecay path and compound decay
// from the original source frame.
func TestPersistCrossFrameFields(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "empirical", Decay: DecayPolicy{Kind: DecayExponential, Halflife: 365 * 24 * time.Hour}})
	s.RegisterFrame(Frame{Name: "reasoning", Decay: DecayPolicy{Kind: DecayExponential, Halflife: 1825 * 24 * time.Hour}})

	if err := s.Assert(&Record{ID: "r1", Frame: "empirical", Content: "empirical finding", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	srcBelief := &Belief{
		ID: "b-empirical", Frame: "empirical", Content: "empirical belief",
		Confidence: 0.8, AssertedAt: now, Derivation: []string{"r1"},
	}
	if err := s.Believe(srcBelief); err != nil {
		t.Fatal(err)
	}
	derived := &Belief{
		ID: "b-reasoning", Frame: "reasoning", Content: "reasoning derived from empirical",
		Confidence: 0.75, AssertedAt: now, Derivation: []string{"b-empirical"},
	}
	if err := s.Believe(derived); err != nil {
		t.Fatal(err)
	}

	// Verify cross-frame snapshot was captured.
	s.mu.RLock()
	bf := s.beliefs["b-reasoning"]
	nCrossFrame := len(bf.CrossFrame)
	s.mu.RUnlock()
	if nCrossFrame == 0 {
		t.Fatal("CrossFrame not captured at assertion time")
	}

	if err := SaveStore(s, db); err != nil {
		t.Fatal(err)
	}
	s2, err := LoadStore(db, now)
	if err != nil {
		t.Fatal(err)
	}

	s2.mu.RLock()
	loaded := s2.beliefs["b-reasoning"]
	s2.mu.RUnlock()
	if loaded == nil {
		t.Fatal("cross-frame belief lost")
	}
	if len(loaded.CrossFrame) != nCrossFrame {
		t.Errorf("CrossFrame length: want %d, got %d", nCrossFrame, len(loaded.CrossFrame))
	}
	if loaded.CrossFrame[0].SourceFrame != "empirical" {
		t.Errorf("CrossFrame source frame: want empirical, got %s", loaded.CrossFrame[0].SourceFrame)
	}
}

// TestPersistDecayOverride verifies per-belief decay overrides survive BoltDB.
func TestPersistDecayOverride(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewStore()
	now := time.Now()
	s.RegisterFrame(Frame{Name: "reasoning", Decay: DecayPolicy{Kind: DecayExponential, Halflife: 365 * 24 * time.Hour}})
	if err := s.Assert(&Record{ID: "r-base", Frame: "reasoning", Content: "base record", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	override := DecayPolicy{Kind: DecayLinear, Rate: 0.01}
	b := &Belief{
		ID: "b-override", Frame: "reasoning", Content: "belief with custom decay",
		Confidence: 0.9, AssertedAt: now, Derivation: []string{"r-base"},
		DecayOverride: &override,
	}
	if err := s.Believe(b); err != nil {
		t.Fatal(err)
	}
	if err := SaveStore(s, db); err != nil {
		t.Fatal(err)
	}
	s2, err := LoadStore(db, now)
	if err != nil {
		t.Fatal(err)
	}
	s2.mu.RLock()
	loaded := s2.beliefs["b-override"]
	s2.mu.RUnlock()
	if loaded == nil {
		t.Fatal("belief with decay override lost")
	}
	if loaded.DecayOverride == nil {
		t.Fatal("DecayOverride lost after BoltDB round-trip")
	}
	if loaded.DecayOverride.Kind != DecayLinear {
		t.Errorf("DecayOverride.Kind: want DecayLinear, got %v", loaded.DecayOverride.Kind)
	}
	if loaded.DecayOverride.Rate != 0.01 {
		t.Errorf("DecayOverride.Rate: want 0.01, got %g", loaded.DecayOverride.Rate)
	}
	// Confidence must use the override at read time, not the frame's policy.
	elapsed := 100 * 24 * time.Hour
	conf := loaded.CurrentConfidence(s2.frames["reasoning"], now.Add(elapsed))
	expected := 0.9 - 0.01*100 // linear: rate per day
	if expected < 0 {
		expected = 0
	}
	if abs := conf - expected; abs > 0.001 || abs < -0.001 {
		t.Errorf("DecayOverride not applied: want ~%.3f, got %.3f", expected, conf)
	}
}
