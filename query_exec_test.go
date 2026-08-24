package lumen

import (
	"math"
	"testing"
	"time"
)

// setupQueryStore builds a store with a realistic revision history for testing.
//
//   r1 ────────────────────────────► b1 (initially 0.60)
//   r2 ──────────────────────────────┤    revised to 0.75 at t1
//   r3 (retracted at t2) ────────────┘    revised to 0.55 at t3
func setupRevisionStore(t *testing.T) (*Store, map[string]time.Time) {
	t.Helper()
	s := NewStore()
	s.RegisterFrame(Frame{Name: "test", Decay: DecayPolicy{Kind: "none"}, Composition: "bayesian"})

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(24 * time.Hour)
	t2 := t0.Add(48 * time.Hour)
	t3 := t0.Add(72 * time.Hour)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	_ = must // suppress unused warning if only used for non-Revise calls

	ts := t0
	must(s.Assert(&Record{ID: "r1", Content: "Record 1", Timestamp: ts, Frame: "test"}))
	must(s.Assert(&Record{ID: "r2", Content: "Record 2", Timestamp: ts, Frame: "test"}))
	must(s.Assert(&Record{ID: "r3", Content: "Record 3 (will be retracted)", Timestamp: ts, Frame: "test"}))

	// Initial belief.
	must(s.Believe(&Belief{
		ID: "b1", Content: "Initial belief", Confidence: 0.60,
		AssertedAt: t0, Frame: "test",
		Derivation: []string{"r1", "r2"},
	}))

	// First revision: confidence 0.60 → 0.75, add r3 as source.
	_, revErr := s.Revise("r1", "Record 1 revised", 0.80, "first revision", t1)
	must(revErr)
	must(s.ReAssert("b1", "Belief after revision 1", 0.75, t1))
	// Manually update derivation to include r3 for source-changes test.
	s.mu.Lock()
	s.beliefs["b1"].Derivation = []string{"r1", "r2", "r3"}
	s.mu.Unlock()

	// Retract r3.
	must(s.Retract("r3", "sensor failure", t2))

	// Second revision: confidence 0.75 → 0.55, remove r3 from sources.
	_, revErr2 := s.Revise("r1", "Record 1 final", 0.90, "second revision", t3)
	must(revErr2)
	must(s.ReAssert("b1", "Belief after revision 2", 0.55, t3))
	s.mu.Lock()
	s.beliefs["b1"].Derivation = []string{"r1", "r2"}
	s.mu.Unlock()

	t4 := t3.Add(time.Hour)
	must(s.Retract("r2", "data corruption", t4))

	return s, map[string]time.Time{"t0": t0, "t1": t1, "t2": t2, "t3": t3, "t4": t4}
}

func TestQueryExecConfidenceChanges(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t3"].Add(time.Hour)

	result, err := s.ExecuteQuery(ParsedQuery{
		ID:     "q1",
		Target: "b1",
		Select: "confidence-changes",
	}, now)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}

	if len(result.Events) != 2 {
		t.Fatalf("expected 2 confidence-change events, got %d", len(result.Events))
	}

	// First change: 0.60 → 0.75
	e0 := result.Events[0]
	if e0.Kind != QueryEventConfidenceChange {
		t.Errorf("event[0] kind: got %s, want confidence-change", e0.Kind)
	}
	if math.Abs(e0.ConfFrom-0.60) > 1e-6 || math.Abs(e0.ConfTo-0.75) > 1e-6 {
		t.Errorf("event[0]: from=%.2f to=%.2f, want from=0.60 to=0.75", e0.ConfFrom, e0.ConfTo)
	}
	if e0.Delta < 0 {
		t.Errorf("event[0] delta should be positive: %.4f", e0.Delta)
	}
	t.Logf("event[0]: %.0f%% → %.0f%%  Δ%.1f%%  at %s", e0.ConfFrom*100, e0.ConfTo*100, e0.Delta*100, e0.At.Format("2006-01-02"))

	// Second change: 0.75 → 0.55
	e1 := result.Events[1]
	if math.Abs(e1.ConfFrom-0.75) > 1e-6 || math.Abs(e1.ConfTo-0.55) > 1e-6 {
		t.Errorf("event[1]: from=%.2f to=%.2f, want from=0.75 to=0.55", e1.ConfFrom, e1.ConfTo)
	}
	if e1.Delta >= 0 {
		t.Errorf("event[1] delta should be negative: %.4f", e1.Delta)
	}
	t.Logf("event[1]: %.0f%% → %.0f%%  Δ%.1f%%  at %s", e1.ConfFrom*100, e1.ConfTo*100, e1.Delta*100, e1.At.Format("2006-01-02"))
}

func TestQueryExecConfidenceChangesWhereFilter(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t3"].Add(time.Hour)

	// Only events where confidence dropped (delta < 0).
	result, err := s.ExecuteQuery(ParsedQuery{
		ID:     "q-drops",
		Target: "b1",
		Select: "confidence-changes",
		Where:  "change < 0",
	}, now)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("expected 1 drop event, got %d", len(result.Events))
	}
	if result.Events[0].Delta >= 0 {
		t.Errorf("expected negative delta, got %.4f", result.Events[0].Delta)
	}

	// Only events where confidence rose by more than 0.10.
	result2, err := s.ExecuteQuery(ParsedQuery{
		ID:     "q-rises",
		Target: "b1",
		Select: "confidence-changes",
		Where:  "change > 0.10",
	}, now)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if len(result2.Events) != 1 {
		t.Fatalf("expected 1 large-rise event, got %d", len(result2.Events))
	}
	t.Logf("rise event: Δ%.1f%%", result2.Events[0].Delta*100)
}

func TestQueryExecSinceFilter(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t3"].Add(time.Hour)

	// Ask only for events after t2 — should exclude the first confidence change.
	result, err := s.ExecuteQuery(ParsedQuery{
		ID:     "q-since",
		Target: "b1",
		Select: "confidence-changes",
		Since:  times["t2"].Format(time.RFC3339),
	}, now)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	// Only the second change (at t3) falls after t2.
	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event after since filter, got %d", len(result.Events))
	}
	if result.Events[0].At.Before(times["t2"]) {
		t.Errorf("event before since boundary: %s", result.Events[0].At)
	}
	t.Logf("since-filtered event at %s", result.Events[0].At.Format("2006-01-02"))
}

func TestQueryExecSourceChanges(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t3"].Add(time.Hour)

	result, err := s.ExecuteQuery(ParsedQuery{
		ID:     "q-src",
		Target: "b1",
		Select: "source-changes",
	}, now)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}

	// We expect: r3 added (at t1), r3 removed (at t3).
	added := 0
	removed := 0
	for _, ev := range result.Events {
		t.Logf("source-change: %s  source=%s  action=%s  at=%s", ev.Kind, ev.SourceID, ev.Action, ev.At.Format("2006-01-02"))
		if ev.Kind == QueryEventSourceAdded && ev.SourceID == "r3" {
			added++
		}
		if ev.Kind == QueryEventSourceRemoved && ev.SourceID == "r3" {
			removed++
		}
	}
	if added != 1 {
		t.Errorf("expected r3 added once, got %d", added)
	}
	if removed != 1 {
		t.Errorf("expected r3 removed once, got %d", removed)
	}
}

func TestQueryExecRetractionEvents(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t4"].Add(time.Hour)

	result, err := s.ExecuteQuery(ParsedQuery{
		ID:     "q-retract",
		Target: "b1",
		Select: "retraction-events",
	}, now)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 retraction event (r2), got %d", len(result.Events))
	}
	ev := result.Events[0]
	if ev.RecordID != "r2" {
		t.Errorf("expected r2 retraction, got %s", ev.RecordID)
	}
	if ev.RetractReason != "data corruption" {
		t.Errorf("expected reason 'data corruption', got %q", ev.RetractReason)
	}
	t.Logf("retraction: record=%s reason=%q at=%s", ev.RecordID, ev.RetractReason, ev.At.Format("2006-01-02"))
}

func TestQueryExecRetractionWhereFilter(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t3"].Add(time.Hour)

	// Filter to retractions whose reason contains "corruption" (matches r2).
	result, err := s.ExecuteQuery(ParsedQuery{
		ID:     "q-corruption",
		Target: "b1",
		Select: "retraction-events",
		Where:  "reason contains corruption",
	}, now)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event matching 'corruption', got %d", len(result.Events))
	}

	// Filter to retractions whose reason contains "radiation" — should return 0.
	result2, err := s.ExecuteQuery(ParsedQuery{
		ID:     "q-radiation",
		Target: "b1",
		Select: "retraction-events",
		Where:  "reason contains radiation",
	}, now)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if len(result2.Events) != 0 {
		t.Errorf("expected 0 events for 'radiation' filter, got %d", len(result2.Events))
	}
}

func TestQueryExecUnknownSelect(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t3"].Add(time.Hour)

	_, err := s.ExecuteQuery(ParsedQuery{
		ID:     "q-bad",
		Target: "b1",
		Select: "what-are-you-on-about",
	}, now)
	if err == nil {
		t.Error("expected error for unknown select kind, got nil")
	}
	t.Logf("expected error: %v", err)
}

func TestQueryExecBeliefNotFound(t *testing.T) {
	s := NewStore()
	_, err := s.ExecuteQuery(ParsedQuery{
		ID:     "q-missing",
		Target: "ghost",
		Select: "confidence-changes",
	}, time.Now())
	if err == nil {
		t.Error("expected error for missing belief, got nil")
	}
}

func TestParseEventPredicate(t *testing.T) {
	tests := []struct {
		where   string
		wantErr bool
		field   string
		op      string
		value   string
	}{
		{"change > 0.1", false, "change", ">", "0.1"},
		{"change <= -0.2", false, "change", "<=", "-0.2"},
		{"reason contains sensor", false, "reason", "contains", "sensor"},
		{"source = r3", false, "source", "=", "r3"},
		{"action != removed", false, "action", "!=", "removed"},
		{"reason contains \"network error\"", false, "reason", "contains", "network error"},
		{"", false, "", "", ""},    // empty = no predicate, ok
		{"change", true, "", "", ""}, // too few tokens
		{"change ??? 0.1", true, "", "", ""},
	}

	for _, tc := range tests {
		p, err := parseEventPredicate(tc.where)
		if tc.wantErr {
			if err == nil {
				t.Errorf("where=%q: expected error, got nil", tc.where)
			}
			continue
		}
		if err != nil {
			t.Errorf("where=%q: unexpected error: %v", tc.where, err)
			continue
		}
		if tc.where == "" {
			if p != nil {
				t.Errorf("empty where should return nil predicate")
			}
			continue
		}
		if p.field != tc.field || p.op != tc.op || p.value != tc.value {
			t.Errorf("where=%q: got field=%s op=%s value=%s, want field=%s op=%s value=%s",
				tc.where, p.field, p.op, p.value, tc.field, tc.op, tc.value)
		}
	}
}

func TestQueryExecRenderOutput(t *testing.T) {
	s, times := setupRevisionStore(t)
	now := times["t3"].Add(time.Hour)

	result, err := s.ExecuteQuery(ParsedQuery{
		ID:     "q-render",
		Target: "b1",
		Select: "confidence-changes",
	}, now)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}

	rendered := RenderArchiveResult(result)
	if rendered == "" {
		t.Error("RenderArchiveResult returned empty string")
	}
	t.Log("\n" + rendered)
}

func TestQueryFromParsedFile(t *testing.T) {
	// Verify that queries declared in .lm files can be extracted and executed.
	src := `
frame philo
    composition: bayesian
    decay: none

record r1 in philo
    "First record."

believe b1 in philo
    "Initial belief."
    confidence: 0.60
    from: r1

query q-history
    target: b1
    select: confidence-changes
    since: "2026-01-01T00:00:00Z"
`
	result, err := ParseFull(src)
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	if len(result.Queries) != 1 {
		t.Fatalf("expected 1 parsed query, got %d", len(result.Queries))
	}
	q := result.Queries[0]
	if q.ID != "q-history" || q.Target != "b1" || q.Select != "confidence-changes" {
		t.Errorf("parsed query mismatch: %+v", q)
	}

	// Load into store and execute.
	s := NewStore()
	now := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(src, s, now); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// No revisions made, so no confidence-change events.
	ar, err := s.ExecuteQuery(q, now)
	if err != nil {
		t.Fatalf("ExecuteQuery: %v", err)
	}
	if len(ar.Events) != 0 {
		t.Errorf("expected 0 events for unrevised belief, got %d", len(ar.Events))
	}
	t.Logf("query %s on unrevised belief: %d events (correct)", q.ID, len(ar.Events))
}

func TestNamedQueryRoundtrip(t *testing.T) {
	// Load a .lm file with query declarations; verify they're stored and executable.
	src := `
frame philo
    composition: bayesian
    decay: none

record r1 in philo
    "Chalmers 1995."
    at: "1995-01-01T00:00:00Z"

believe hard-problem in philo
    "The hard problem is real."
    confidence: 0.72
    from: r1

query recent-changes
    target: hard-problem
    select: confidence-changes
    since: "2024-01-01T00:00:00Z"
    where: change > 0.05

query all-retractions
    target: hard-problem
    select: retraction-events
`
	s := NewStore()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	if err := LoadFile(src, s, now); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// Both queries should be registered.
	all := s.AllQueries()
	if len(all) != 2 {
		t.Fatalf("expected 2 named queries, got %d", len(all))
	}
	t.Logf("named queries: %v", func() []string {
		ids := make([]string, len(all)); for i, q := range all { ids[i] = q.ID }; return ids
	}())

	// Execute by ID.
	ar, err := s.RunQueryByID("recent-changes", now)
	if err != nil {
		t.Fatalf("RunQueryByID: %v", err)
	}
	// No revisions → no events.
	if len(ar.Events) != 0 {
		t.Errorf("expected 0 events for unrevised belief, got %d", len(ar.Events))
	}
	if ar.QueryID != "recent-changes" {
		t.Errorf("QueryID: got %s", ar.QueryID)
	}

	// Unknown query ID returns a helpful error.
	_, err = s.RunQueryByID("does-not-exist", now)
	if err == nil {
		t.Error("expected error for unknown query ID, got nil")
	}
	t.Logf("unknown query error: %v", err)

	// GetQuery works.
	q, ok := s.GetQuery("all-retractions")
	if !ok {
		t.Error("GetQuery returned false for existing query")
	}
	if q.Target != "hard-problem" || q.Select != "retraction-events" {
		t.Errorf("GetQuery returned wrong query: %+v", q)
	}
}

func TestAllQueriesSorted(t *testing.T) {
	s := NewStore()
	s.AddQuery(ParsedQuery{ID: "z-query", Target: "b", Select: "confidence-changes"})
	s.AddQuery(ParsedQuery{ID: "a-query", Target: "b", Select: "confidence-changes"})
	s.AddQuery(ParsedQuery{ID: "m-query", Target: "b", Select: "confidence-changes"})

	all := s.AllQueries()
	if len(all) != 3 {
		t.Fatalf("expected 3 queries, got %d", len(all))
	}
	if all[0].ID != "a-query" || all[1].ID != "m-query" || all[2].ID != "z-query" {
		t.Errorf("queries not sorted: %v", func() []string {
			ids := make([]string, len(all)); for i, q := range all { ids[i] = q.ID }; return ids
		}())
	}
}
