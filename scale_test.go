package lumen

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// buildScaleStore generates a realistic store: a small set of frames, records
// with entity-bearing content, and beliefs whose derivation chains follow a
// power-law-ish depth distribution (most shallow, some deep) — the shape an
// agent session produces over time.
func buildScaleStore(nBeliefs int, seed int64) (*Store, time.Time) {
	rng := rand.New(rand.NewSource(seed))
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	s := NewStore()

	frames := []string{"empirical", "reasoning", "retrieved", "contemporary"}
	for _, f := range frames {
		s.RegisterFrame(Frame{Name: f, Decay: DecayPolicy{Kind: DecayExponential, Halflife: 365 * 24 * time.Hour}})
	}

	subjects := []string{"Consciousness", "Memory", "Attention", "Language", "Perception",
		"GlobalWorkspace", "Integration", "Prediction", "Embodiment", "Qualia",
		"Retrodiction", "Decay", "Provenance", "Calibration", "Evidence"}
	verbs := []string{"requires", "modulates", "predicts", "constrains", "explains",
		"correlates with", "does not require", "fails to explain"}

	nRecords := nBeliefs / 4
	recIDs := make([]string, 0, nRecords)
	for i := 0; i < nRecords; i++ {
		id := fmt.Sprintf("rec-%d", i)
		subj := subjects[rng.Intn(len(subjects))]
		obj := subjects[rng.Intn(len(subjects))]
		content := fmt.Sprintf("Study %d found that %s %s %s under condition %d.",
			i, subj, verbs[rng.Intn(len(verbs))], obj, rng.Intn(100))
		ts := now.Add(-time.Duration(rng.Intn(720)) * 24 * time.Hour)
		if err := s.Assert(&Record{ID: id, Frame: frames[rng.Intn(len(frames))], Content: content, Timestamp: ts}); err != nil {
			panic(err)
		}
		recIDs = append(recIDs, id)
	}

	belIDs := make([]string, 0, nBeliefs)
	for i := 0; i < nBeliefs; i++ {
		id := fmt.Sprintf("bel-%d", i)
		subj := subjects[rng.Intn(len(subjects))]
		obj := subjects[rng.Intn(len(subjects))]
		content := fmt.Sprintf("%s %s %s in the context of %s.",
			subj, verbs[rng.Intn(len(verbs))], obj, subjects[rng.Intn(len(subjects))])

		// Derivation: 60% from records only, 30% mixed, 10% deep belief chains.
		var deriv []string
		switch {
		case i < 10 || rng.Float64() < 0.6:
			for k := 0; k < 1+rng.Intn(2); k++ {
				deriv = append(deriv, recIDs[rng.Intn(len(recIDs))])
			}
		case rng.Float64() < 0.75:
			deriv = append(deriv, recIDs[rng.Intn(len(recIDs))], belIDs[rng.Intn(len(belIDs))])
		default:
			for k := 0; k < 2+rng.Intn(3); k++ {
				deriv = append(deriv, belIDs[rng.Intn(len(belIDs))])
			}
		}
		ts := now.Add(-time.Duration(rng.Intn(365)) * 24 * time.Hour)
		if err := s.Believe(&Belief{
			ID: id, Frame: frames[rng.Intn(len(frames))], Content: content,
			Confidence: 0.3 + rng.Float64()*0.65, AssertedAt: ts, Derivation: deriv,
		}); err != nil {
			panic(err)
		}
		belIDs = append(belIDs, id)
	}
	return s, now
}

// TestScaleSmoke keeps the generator itself honest at small scale.
func TestScaleSmoke(t *testing.T) {
	s, now := buildScaleStore(200, 1)
	if got := len(s.AllBeliefs(now)); got != 200 {
		t.Fatalf("want 200 beliefs, got %d", got)
	}
}

func benchAt(b *testing.B, n int, op func(s *Store, now time.Time)) {
	s, now := buildScaleStore(n, 42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		op(s, now)
	}
}

func BenchmarkScaleAllBeliefs1k(b *testing.B)  { benchAt(b, 1000, func(s *Store, now time.Time) { s.AllBeliefs(now) }) }
func BenchmarkScaleAllBeliefs10k(b *testing.B) { benchAt(b, 10000, func(s *Store, now time.Time) { s.AllBeliefs(now) }) }

func BenchmarkScaleConflictScan1k(b *testing.B) { benchAt(b, 1000, func(s *Store, now time.Time) { s.ConflictScan(now) }) }

func BenchmarkScaleBeliefHealth1k(b *testing.B) {
	benchAt(b, 1000, func(s *Store, now time.Time) { _, _ = s.BeliefHealth("bel-500", now) })
}

func BenchmarkScaleStoreHealth1k(b *testing.B) { benchAt(b, 1000, func(s *Store, now time.Time) { s.StoreHealth(now) }) }

func BenchmarkScaleSearchBuild10k(b *testing.B) { benchAt(b, 10000, func(s *Store, now time.Time) { s.BuildSearchIndex() }) }

func BenchmarkScaleSearchQuery10k(b *testing.B) {
	s, now := buildScaleStore(10000, 42)
	idx := s.BuildSearchIndex()
	_ = now
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Search(idx, "Consciousness requires Integration", 10)
	}
}

func BenchmarkScaleFragility1k(b *testing.B) { benchAt(b, 1000, func(s *Store, now time.Time) { s.FragilityScan(now) }) }

func BenchmarkScaleQueryOne10k(b *testing.B) {
	benchAt(b, 10000, func(s *Store, now time.Time) { _, _ = s.Query("bel-5000", now) })
}

// BenchmarkScaleConflictScanWarm measures the cached path — after the first
// scan populates the cache, subsequent calls should be O(1) copy.
func BenchmarkScaleConflictScanWarm(b *testing.B) {
	s, now := buildScaleStore(1000, 42)
	s.ConflictScan(now) // warm the cache
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.ConflictScan(now)
	}
}

// BenchmarkScaleConflictScanCold forces a dirty cache on every iteration
// by inserting a dummy belief, measuring the actual O(n²) scan.
func BenchmarkScaleConflictScanCold1k(b *testing.B) {
	s, now := buildScaleStore(1000, 42)
	// Pre-register a frame for the dummies.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Invalidate by adding a record, which calls invalidateConflicts.
		_ = s.Assert(&Record{
			ID: fmt.Sprintf("dummy-cold-%d", i), Frame: "empirical",
			Content: fmt.Sprintf("dummy %d", i), Timestamp: now,
		})
		b.StartTimer()
		s.ConflictScan(now)
	}
}

func BenchmarkScaleConflictScanCold10k(b *testing.B) {
	s, now := buildScaleStore(10000, 42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		_ = s.Assert(&Record{
			ID: fmt.Sprintf("dummy-cold10k-%d", i), Frame: "empirical",
			Content: fmt.Sprintf("dummy %d", i), Timestamp: now,
		})
		b.StartTimer()
		s.ConflictScan(now)
	}
}

func BenchmarkScaleCachedSearchCold10k(b *testing.B) {
	s, _ := buildScaleStore(10000, 42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		_ = s.Assert(&Record{
			ID: fmt.Sprintf("dummy-search-%d", i), Frame: "empirical",
			Content: fmt.Sprintf("dummy belief record %d about Consciousness", i),
			Timestamp: time.Now(),
		})
		b.StartTimer()
		s.CachedSearch("Consciousness requires Integration", 10)
	}
}

func BenchmarkScaleCachedSearchWarm10k(b *testing.B) {
	s, _ := buildScaleStore(10000, 42)
	s.CachedSearch("Consciousness", 10) // warm
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.CachedSearch("Consciousness requires Integration", 10)
	}
}

func BenchmarkScaleFragility10k(b *testing.B) { benchAt(b, 10000, func(s *Store, now time.Time) { s.FragilityScan(now) }) }
