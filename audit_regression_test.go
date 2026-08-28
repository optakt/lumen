package lumen

import (
	"math"
	"slices"
	"testing"
	"time"
)

func auditStore(t *testing.T, decay DecayPolicy) (*Store, time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	s := NewStore()
	s.RegisterFrame(Frame{Name: "f", Composition: CompositionBayesian, Decay: decay})
	return s, now
}

func TestBelieveFailureIsAtomic(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	if err := s.Assert(&Record{ID: "r1", Frame: "f", Content: "source", Timestamp: now}); err != nil {
		t.Fatal(err)
	}

	err := s.Believe(&Belief{
		ID: "b1", Frame: "f", Content: "claim", Confidence: 0.8,
		AssertedAt: now, Derivation: []string{"r1", "missing"},
	})
	if err == nil {
		t.Fatal("expected missing-source error")
	}
	if got := s.Graph.ReachableByDerivation("r1"); slices.Contains(got, "b1") {
		t.Fatalf("failed Believe left derivation edge behind: %v", got)
	}
	if deps := s.dependents["r1"]; deps != nil && deps["b1"] {
		t.Fatal("failed Believe left dependents entry behind")
	}
	if _, err := s.Query("b1", now); err == nil {
		t.Fatal("failed belief was stored")
	}
}

func TestConflictCacheAccountsForDecayTime(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayExponential, Halflife: 24 * time.Hour})
	s.Entities.RegisterEntity(&Entity{ID: "Alpha"})
	for _, b := range []*Belief{
		{ID: "high", Frame: "f", Content: "Alpha remains strongly supported", Confidence: 0.95, AssertedAt: now},
		{ID: "low", Frame: "f", Content: "Alpha has weak support", Confidence: 0.20, AssertedAt: now},
	} {
		if err := s.Believe(b); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.ConflictScan(now); len(got) == 0 {
		t.Fatal("expected initial confidence-divergence conflict")
	}
	if got := s.ConflictScan(now.Add(48 * time.Hour)); len(got) != 0 {
		t.Fatalf("decayed confidences no longer diverge, but cached conflicts remain: %+v", got)
	}
}

func TestReAssertInvalidatesSearchAndEntityIndex(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	s.Entities.RegisterEntity(&Entity{ID: "Alpha"})
	s.Entities.RegisterEntity(&Entity{ID: "Beta"})
	if err := s.Believe(&Belief{ID: "b", Frame: "f", Content: "Alpha claim", Confidence: 0.8, AssertedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.Assert(&Record{ID: "other", Frame: "f", Content: "Gamma observation", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	if got := s.CachedSearch("Alpha", 10); len(got) != 1 {
		t.Fatalf("warm Alpha search: got %d results", len(got))
	}
	if err := s.ReAssert("b", "Beta claim", 0.9, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := s.CachedSearch("Alpha", 10); len(got) != 0 {
		t.Fatalf("stale search index still returns old content: %+v", got)
	}
	if got := s.CachedSearch("Beta", 10); len(got) != 1 || got[0].NodeID != "b" {
		t.Fatalf("updated content missing from search: %+v", got)
	}
	if got := s.Entities.EntitiesForNode("b"); !slices.Equal(got, []string{"beta"}) {
		t.Fatalf("entity index not refreshed after content update: %v", got)
	}
}

func TestEntityRemoveClearsBothIndexes(t *testing.T) {
	g := NewEntityGraph()
	g.RegisterEntity(&Entity{ID: "Alpha"})
	g.ExtractAndIndex("b", "Alpha claim")
	g.Remove("b")
	if got := g.EntitiesForNode("b"); len(got) != 0 {
		t.Fatalf("node-to-entity index retained removed node: %v", got)
	}
	if got := g.NodesForEntity("Alpha"); len(got) != 0 {
		t.Fatalf("entity-to-node index retained removed node: %v", got)
	}
}

func TestTemporalAndVersionSnapshotsAreDefensiveCopies(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tg := NewTemporalGraph()
	tg.Record("b", "belief", now, []string{"r"})
	timeline := tg.Timeline()
	timeline[0].EnabledBy[0] = "mutated"
	if got := tg.EnabledBy("b"); !slices.Equal(got, []string{"r"}) {
		t.Fatalf("Timeline exposed internal EnabledBy slice: %v", got)
	}

	vs := NewVersionStore()
	b := &Belief{ID: "b", Content: "original", Derivation: []string{"r"}, AssertedAt: now}
	vs.Snapshot(b, now.Add(time.Hour), "change")
	h := vs.History("b")
	h[0].Derivation[0] = "mutated"
	if got := vs.History("b")[0].Derivation; !slices.Equal(got, []string{"r"}) {
		t.Fatalf("History exposed internal Derivation slice: %v", got)
	}
	v := vs.VersionAt("b", now.Add(2*time.Hour))
	v.Content = "mutated"
	if got := vs.VersionAt("b", now.Add(2*time.Hour)).Content; got != "original" {
		t.Fatalf("VersionAt exposed internal version: %q", got)
	}
}

func TestMergeRetirementInvalidatesSearch(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	for _, b := range []*Belief{
		{ID: "a", Frame: "f", Content: "Alpha evidence", Confidence: 0.7, AssertedAt: now},
		{ID: "b", Frame: "f", Content: "Beta evidence", Confidence: 0.8, AssertedAt: now},
	} {
		if err := s.Believe(b); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.CachedSearch("Alpha", 10); len(got) != 1 {
		t.Fatalf("warm search: %+v", got)
	}
	if _, err := s.MergeBeliefs("a", "b", "merged", "Combined result", "f", "average", true, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := s.CachedSearch("Alpha", 10); len(got) != 0 {
		t.Fatalf("retired belief remains in cached search: %+v", got)
	}
}

func TestRecoveryInvalidatesSearch(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	if err := s.Assert(&Record{ID: "r", Frame: "f", Content: "source", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.Assert(&Record{ID: "other", Frame: "f", Content: "Gamma observation", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.Believe(&Belief{ID: "b", Frame: "f", Content: "Alpha conclusion", Confidence: 0.8, AssertedAt: now, Derivation: []string{"r"}}); err != nil {
		t.Fatal(err)
	}
	result, err := s.MinimalContraction("r", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyContraction(result, "bad source", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := s.CachedSearch("Alpha", 10); len(got) != 0 {
		t.Fatalf("contracted belief should be absent: %+v", got)
	}
	s.mu.Lock()
	s.records["r"].Retracted = false
	s.mu.Unlock()
	if err := s.Recover("b", now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := s.CachedSearch("Alpha", 10); len(got) != 1 || got[0].NodeID != "b" {
		t.Fatalf("recovered belief missing from cached search: %+v", got)
	}
}

func TestReAssertRejectsTerminalSupersession(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	for _, b := range []*Belief{
		{ID: "a", Frame: "f", Content: "A", Confidence: 0.7, AssertedAt: now},
		{ID: "b", Frame: "f", Content: "B", Confidence: 0.8, AssertedAt: now},
	} {
		if err := s.Believe(b); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.MergeBeliefs("a", "b", "merged", "M", "f", "average", true, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.ReAssert("a", "resurrected", 0.9, now.Add(2*time.Hour)); err == nil {
		t.Fatal("ReAssert resurrected a terminally superseded belief")
	}
}

func TestReviseRefreshesSearchAndEntityIndex(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	s.Entities.RegisterEntity(&Entity{ID: "Alpha"})
	s.Entities.RegisterEntity(&Entity{ID: "Beta"})
	if err := s.Assert(&Record{ID: "r", Frame: "f", Content: "Alpha observation", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.Assert(&Record{ID: "other", Frame: "f", Content: "Gamma observation", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	if got := s.CachedSearch("Alpha", 10); len(got) != 1 {
		t.Fatalf("warm search: %+v", got)
	}
	if _, err := s.Revise("r", "Beta observation", 0, "correction", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := s.CachedSearch("Alpha", 10); len(got) != 0 {
		t.Fatalf("revised record remains under old search term: %+v", got)
	}
	if got := s.CachedSearch("Beta", 10); len(got) != 1 || got[0].NodeID != "r" {
		t.Fatalf("revised record missing under new search term: %+v", got)
	}
	if got := s.Entities.EntitiesForNode("r"); !slices.Equal(got, []string{"beta"}) {
		t.Fatalf("record entity index not refreshed: %v", got)
	}
}

func TestSaveStoreWithBridgeCompletesAndRoundTrips(t *testing.T) {
	s, _ := auditStore(t, DecayPolicy{Kind: DecayNone})
	if err := s.Bridges.Register(&Bridge{Name: "f-to-g", FromFrame: "f", ToFrame: "g", Loss: "precision"}); err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(t.TempDir() + "/bridge.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	done := make(chan error, 1)
	go func() { done <- SaveStore(s, db) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SaveStore deadlocked while persisting a bridge")
	}
	loaded, err := LoadStore(db, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Bridges.Lookup("f-to-g"); !ok {
		t.Fatal("bridge did not survive persistence round-trip")
	}
}

func TestSnapshotBeforeRetractionRestoresHistoricalState(t *testing.T) {
	s, t0 := auditStore(t, DecayPolicy{Kind: DecayNone})
	if err := s.Assert(&Record{ID: "r", Frame: "f", Content: "source", Timestamp: t0}); err != nil {
		t.Fatal(err)
	}
	if err := s.Believe(&Belief{ID: "b", Frame: "f", Content: "claim", Confidence: 0.8, AssertedAt: t0, Derivation: []string{"r"}}); err != nil {
		t.Fatal(err)
	}
	retractedAt := t0.Add(2 * time.Hour)
	if err := s.Retract("r", "bad source", retractedAt); err != nil {
		t.Fatal(err)
	}

	before := s.SnapshotAt(t0.Add(time.Hour))
	before.mu.RLock()
	recordRetracted := before.records["r"].Retracted
	before.mu.RUnlock()
	if recordRetracted {
		t.Fatal("snapshot before retraction reports record as retracted")
	}
	result, err := before.Query("b", t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.State != BeliefActive {
		t.Fatalf("snapshot before retraction reports belief state %v, want active", result.State)
	}
}

func TestSnapshotBeforeReAssertRestoresPriorBelief(t *testing.T) {
	s, t0 := auditStore(t, DecayPolicy{Kind: DecayNone})
	if err := s.Believe(&Belief{ID: "b", Frame: "f", Content: "original", Confidence: 0.6, AssertedAt: t0}); err != nil {
		t.Fatal(err)
	}
	changedAt := t0.Add(2 * time.Hour)
	if err := s.ReAssert("b", "updated", 0.9, changedAt); err != nil {
		t.Fatal(err)
	}

	before := s.SnapshotAt(t0.Add(time.Hour))
	result, err := before.Query("b", t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "original" || result.CurrentConfidence != 0.6 {
		t.Fatalf("snapshot leaked later re-assertion: content=%q confidence=%g", result.Content, result.CurrentConfidence)
	}
	after := s.SnapshotAt(changedAt)
	result, err = after.Query("b", changedAt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "updated" || result.CurrentConfidence != 0.9 {
		t.Fatalf("snapshot at change time missed re-assertion: content=%q confidence=%g", result.Content, result.CurrentConfidence)
	}
}

func TestSnapshotPreservesSemanticAndEntityGraphs(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	s.Entities.RegisterEntity(&Entity{ID: "Alpha"})
	for _, b := range []*Belief{
		{ID: "a", Frame: "f", Content: "Alpha first", Confidence: 0.7, AssertedAt: now},
		{ID: "b", Frame: "f", Content: "Alpha second", Confidence: 0.8, AssertedAt: now},
	} {
		if err := s.Believe(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Reference("a", "b", EdgeContrasts, "opposed"); err != nil {
		t.Fatal(err)
	}
	snap := s.SnapshotAt(now)
	if got := snap.Graph.SemanticNeighbors("a"); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("snapshot lost semantic graph edge: %v", got)
	}
	if got := snap.Entities.NodesForEntity("Alpha"); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("snapshot lost entity graph: %v", got)
	}
}

func TestStoreRejectsInvalidPublicInputs(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	if err := s.Assert(nil); err == nil {
		t.Fatal("Assert(nil) should return an error")
	}
	if err := s.Assert(&Record{Frame: "f", Content: "missing ID", Timestamp: now}); err == nil {
		t.Fatal("record with empty ID should be rejected")
	}
	if err := s.Believe(nil); err == nil {
		t.Fatal("Believe(nil) should return an error")
	}
	for _, confidence := range []float64{-0.1, 1.1} {
		if err := s.Believe(&Belief{
			ID: "bad", Frame: "f", Content: "invalid confidence",
			Confidence: confidence, AssertedAt: now,
		}); err == nil {
			t.Fatalf("confidence %g should be rejected", confidence)
		}
	}
}

func TestConfidenceDoesNotGrowBeforeAssertion(t *testing.T) {
	policy := DecayPolicy{Kind: DecayExponential, Halflife: 24 * time.Hour}
	assertedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	belief := &Belief{Confidence: 0.8, AssertedAt: assertedAt}
	if got := belief.CurrentConfidence(Frame{Decay: policy}, assertedAt.Add(-24*time.Hour)); got != 0.8 {
		t.Fatalf("negative elapsed time increased confidence to %g", got)
	}
}

func TestRecoverRejectsMissingDerivationSource(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	s.beliefs["b"] = &Belief{
		ID: "b", Frame: "f", Content: "contracted", Confidence: 0.8,
		AssertedAt: now, State: BeliefSuperseded, ContractedBy: "r",
		Derivation: []string{"r", "missing"},
	}
	s.records["r"] = &Record{ID: "r", Frame: "f", Content: "restored", Timestamp: now}
	if err := s.Recover("b", now.Add(time.Hour)); err == nil {
		t.Fatal("Recover accepted an unknown derivation source")
	}
}

func TestBridgeRegistryDoesNotExposeMutableInternals(t *testing.T) {
	registry := NewBridgeRegistry()
	input := &Bridge{Name: "a-to-b", FromFrame: "a", ToFrame: "b", Loss: "precision"}
	if err := registry.Register(input); err != nil {
		t.Fatal(err)
	}
	input.Loss = "mutated input"
	lookup, ok := registry.Lookup("a-to-b")
	if !ok || lookup.Loss != "precision" {
		t.Fatalf("registration retained caller pointer: %+v", lookup)
	}
	lookup.Loss = "mutated output"
	again, _ := registry.Lookup("a-to-b")
	if again.Loss != "precision" {
		t.Fatalf("Lookup exposed internal pointer: %+v", again)
	}
	all := registry.All()
	all[0].Loss = "mutated all"
	again, _ = registry.Lookup("a-to-b")
	if again.Loss != "precision" {
		t.Fatalf("All exposed internal pointer: %+v", again)
	}
}

func TestStoreTakesOwnershipCopies(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	record := &Record{ID: "r", Frame: "f", Content: "original record", Timestamp: now}
	if err := s.Assert(record); err != nil {
		t.Fatal(err)
	}
	record.Content = "mutated record"
	if got := s.ContentFor("r"); got != "original record" {
		t.Fatalf("record changed through caller pointer: %q", got)
	}

	belief := &Belief{
		ID: "b", Frame: "f", Content: "original belief", Confidence: 0.8,
		AssertedAt: now, Derivation: []string{"r"},
	}
	if err := s.Believe(belief); err != nil {
		t.Fatal(err)
	}
	belief.Content = "mutated belief"
	belief.Confidence = 0.1
	belief.Derivation[0] = "missing"
	result, err := s.Query("b", now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "original belief" || result.CurrentConfidence != 0.8 {
		t.Fatalf("belief changed through caller pointer: %+v", result)
	}
	if got := s.Graph.ReachableByDerivation("r"); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("stored derivation changed through caller slice: %v", got)
	}
}

func TestDerivationSourcesUsesInboundEdges(t *testing.T) {
	g := NewBeliefGraph()
	g.AddEdge(Edge{From: "source", To: "belief", Kind: EdgeDerives})
	if got := g.DerivationSources("belief"); !slices.Equal(got, []string{"source"}) {
		t.Fatalf("DerivationSources returned %v", got)
	}
	if got := g.DerivationDependents("source"); !slices.Equal(got, []string{"belief"}) {
		t.Fatalf("DerivationDependents returned %v", got)
	}
}

func TestLoadRejectsUnknownFrameModes(t *testing.T) {
	for name, src := range map[string]string{
		"composition":  "frame f\n    composition: magical\n",
		"stale action": "frame f\n    on_stale_derivation: eventually\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := LoadFile(src, NewStore(), time.Now()); err == nil {
				t.Fatal("unknown frame mode was silently replaced with a default")
			}
		})
	}
}

func TestLoadRejectsMissingRetractionTarget(t *testing.T) {
	src := `frame f
    decay: none

retract missing reason: "not present"
`
	if err := LoadFile(src, NewStore(), time.Now()); err == nil {
		t.Fatal("missing retraction target was silently ignored")
	}
}

func TestCorrelationAwareRetirementUsesTerminalState(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	if err := s.Assert(&Record{ID: "r", Frame: "f", Content: "shared", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	for _, b := range []*Belief{
		{ID: "a", Frame: "f", Content: "Alpha claim", Confidence: 0.7, AssertedAt: now, Derivation: []string{"r"}},
		{ID: "b", Frame: "f", Content: "Beta claim", Confidence: 0.8, AssertedAt: now, Derivation: []string{"r"}},
	} {
		if err := s.Believe(b); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CorrelationAwareMerge("a", "b", "merged", "Combined", "f", true, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		s.mu.RLock()
		belief := s.beliefs[id]
		s.mu.RUnlock()
		if belief.State != BeliefSuperseded {
			t.Fatalf("%s state=%v, want terminal superseded", id, belief.State)
		}
		if belief.Content[0] == '[' {
			t.Fatalf("%s content was destructively prefixed: %q", id, belief.Content)
		}
	}
	s.mu.RLock()
	derivation := append([]string(nil), s.beliefs["merged"].Derivation...)
	s.mu.RUnlock()
	if !slices.Equal(derivation, []string{"a", "b"}) {
		t.Fatalf("merged belief has redundant direct and transitive sources: %v", derivation)
	}
}

func TestProbabilityAPIsRejectNonFiniteAndInvalidMasses(t *testing.T) {
	if _, err := BayesianCompose(0.5, []Evidence{{SourceID: "nan", Confidence: 1, LikelihoodRatio: math.NaN()}}); err == nil {
		t.Fatal("BayesianCompose accepted NaN likelihood ratio")
	}
	mass := DempsterShaferMass{SourceID: "bad", MassTrue: 1.2, MassFalse: -0.2, MassUnknown: 0}
	if err := mass.Normalize(); err == nil {
		t.Fatal("Normalize accepted negative/out-of-range masses that sum to one")
	}
	valid := DempsterShaferMass{SourceID: "valid", MassTrue: 0.6, MassFalse: 0.2, MassUnknown: 0.2}
	if _, _, _, err := DempsterShaferCompose(mass, valid); err == nil {
		t.Fatal("DempsterShaferCompose accepted an invalid mass function")
	}
	if _, err := CredalBayesUpdate(
		CredalPrior{Lo: 0.4, Hi: 0.6},
		[]CredalEvidence{{SourceID: "nan", Confidence: 1, LRLo: math.NaN(), LRHi: 2}},
	); err == nil {
		t.Fatal("CredalBayesUpdate accepted NaN interval bound")
	}
}

func TestImpactScanCombinesDiamondCascade(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	if err := s.Assert(&Record{ID: "r", Frame: "f", Content: "root", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	for _, b := range []*Belief{
		{ID: "left", Frame: "f", Content: "left", Confidence: 0.8, AssertedAt: now, Derivation: []string{"r"}},
		{ID: "right", Frame: "f", Content: "right", Confidence: 0.7, AssertedAt: now, Derivation: []string{"r"}},
		{ID: "top", Frame: "f", Content: "top", Confidence: 0.9, AssertedAt: now, Derivation: []string{"left", "right"}},
	} {
		if err := s.Believe(b); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := s.ImpactScan("r", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.BeliefID == "top" {
			if entry.EstimatedConf != 0 {
				t.Fatalf("diamond cascade applied only one affected parent: %+v", entry)
			}
			return
		}
	}
	t.Fatal("top belief missing from impact scan")
}

func TestHistoricalAnalysisRejectsInvalidSampling(t *testing.T) {
	s, now := auditStore(t, DecayPolicy{Kind: DecayNone})
	if err := s.Believe(&Belief{ID: "b", Frame: "f", Content: "claim", Confidence: 0.8, AssertedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WhatChangedMyMind("b", now, now.Add(time.Hour), 0.1, 0); err == nil {
		t.Fatal("zero samples should return an error")
	}
	if _, err := s.WhatChangedMyMind("b", now.Add(time.Hour), now, 0.1, 10); err == nil {
		t.Fatal("reversed time range should return an error")
	}
}

func TestRenderBiographyHandlesMissingHealth(t *testing.T) {
	bio := &BeliefBiography{BeliefID: "b", Content: "claim", Frame: "f", AssertedAt: time.Now()}
	_ = RenderBiography(bio)
}

func TestArchaeologyEventsUseTheChangeBeingApplied(t *testing.T) {
	s, t0 := auditStore(t, DecayPolicy{Kind: DecayNone})
	if err := s.Believe(&Belief{ID: "b", Frame: "f", Content: "claim", Confidence: 0.6, AssertedAt: t0, Derivation: nil}); err != nil {
		t.Fatal(err)
	}
	t1 := t0.Add(time.Hour)
	t2 := t0.Add(2 * time.Hour)
	s.versions.versions["b"] = []BeliefVersion{
		{Version: 1, Confidence: 0.5, Derivation: []string{"r1"}, AssertedAt: t0, ChangedAt: t1, ChangeReason: "first change"},
		{Version: 2, Confidence: 0.8, Derivation: []string{"r1", "r2"}, AssertedAt: t1, ChangedAt: t2, ChangeReason: "second change"},
	}
	s.mu.Lock()
	s.beliefs["b"].Derivation = []string{"r2"}
	s.mu.Unlock()

	confidence, err := s.execConfidenceChanges("b", t2)
	if err != nil {
		t.Fatal(err)
	}
	if len(confidence) != 2 {
		t.Fatalf("confidence events: %+v", confidence)
	}
	if !confidence[0].At.Equal(t1) || confidence[0].Reason != "first change" || confidence[0].ConfFrom != 0.5 || confidence[0].ConfTo != 0.8 {
		t.Fatalf("first confidence event is shifted to the next change: %+v", confidence[0])
	}
	if !confidence[1].At.Equal(t2) || confidence[1].Reason != "second change" || confidence[1].ConfFrom != 0.8 || confidence[1].ConfTo != 0.6 {
		t.Fatalf("second confidence event is wrong: %+v", confidence[1])
	}

	sources, err := s.execSourceChanges("b", t2)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("source events: %+v", sources)
	}
	for _, event := range sources {
		if event.SourceID == "r2" && event.Action == "added" && !event.At.Equal(t1) {
			t.Fatalf("source addition timestamp shifted to next change: %+v", event)
		}
		if event.SourceID == "r1" && event.Action == "removed" && !event.At.Equal(t2) {
			t.Fatalf("source removal timestamp wrong: %+v", event)
		}
	}
}

func TestPersistenceKeepsSemanticEntityAndTemporalState(t *testing.T) {
	s, t0 := auditStore(t, DecayPolicy{Kind: DecayNone})
	s.Entities.RegisterEntity(&Entity{ID: "Alpha"})
	for _, b := range []*Belief{
		{ID: "a", Frame: "f", Content: "Alpha first", Confidence: 0.7, AssertedAt: t0},
		{ID: "b", Frame: "f", Content: "Alpha original", Confidence: 0.8, AssertedAt: t0},
	} {
		if err := s.Believe(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Reference("a", "b", EdgeContrasts, "opposed"); err != nil {
		t.Fatal(err)
	}
	changedAt := t0.Add(2 * time.Hour)
	if err := s.ReAssert("b", "Alpha updated", 0.9, changedAt); err != nil {
		t.Fatal(err)
	}

	db, err := OpenDB(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := SaveStore(s, db); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadStore(db, changedAt)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Graph.SemanticNeighbors("a"); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("semantic edge lost on restart: %v", got)
	}
	if got := loaded.Entities.NodesForEntity("Alpha"); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("entity index lost on restart: %v", got)
	}
	before := loaded.SnapshotAt(t0.Add(time.Hour))
	result, err := before.Query("b", t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "Alpha original" || result.CurrentConfidence != 0.8 {
		t.Fatalf("temporal history lost on restart: %+v", result)
	}
}

func TestSnapshotBeforeRevisionRestoresRecordContent(t *testing.T) {
	s, t0 := auditStore(t, DecayPolicy{Kind: DecayNone})
	if err := s.Assert(&Record{ID: "r", Frame: "f", Content: "original finding", Timestamp: t0}); err != nil {
		t.Fatal(err)
	}
	changedAt := t0.Add(2 * time.Hour)
	if _, err := s.Revise("r", "corrected finding", 0, "typo", changedAt); err != nil {
		t.Fatal(err)
	}
	before := s.SnapshotAt(t0.Add(time.Hour))
	if got := before.ContentFor("r"); got != "original finding" {
		t.Fatalf("snapshot before revision shows revised content: %q", got)
	}
	after := s.SnapshotAt(changedAt)
	if got := after.ContentFor("r"); got != "corrected finding" {
		t.Fatalf("snapshot at revision time lost new content: %q", got)
	}
}

func TestRecordVersionsSurvivePersistence(t *testing.T) {
	s, t0 := auditStore(t, DecayPolicy{Kind: DecayNone})
	if err := s.Assert(&Record{ID: "r", Frame: "f", Content: "original", Timestamp: t0}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Revise("r", "revised", 0, "update", t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(t.TempDir() + "/rv.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := SaveStore(s, db); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadStore(db, t0.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	before := loaded.SnapshotAt(t0.Add(time.Hour))
	if got := before.ContentFor("r"); got != "original" {
		t.Fatalf("record version history lost on restart: %q", got)
	}
}
