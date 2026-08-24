package lumen

import (
	"fmt"
	"testing"
	"time"
)

// buildBenchStore creates a store with n beliefs in a linear chain:
// r0 → b0 → b1 → b2 → ... → b(n-1).
// This gives us a realistic depth for provenance chain benchmarks.
func buildBenchStore(b *testing.B, n int) (*Store, time.Time) {
	b.Helper()
	s := NewStore()
	s.RegisterFrame(Frame{Name: "bench", Decay: DecayPolicy{Kind: DecayExponential, Halflife: 365 * 24 * time.Hour}})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := s.Assert(&Record{ID: "r0", Content: "Base record.", Frame: "bench", Timestamp: now}); err != nil {
		b.Fatalf("Assert: %v", err)
	}
	// b0 derives from r0; each subsequent belief derives from the previous.
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("b%d", i)
		var src string
		if i == 0 {
			src = "r0"
		} else {
			src = fmt.Sprintf("b%d", i-1)
		}
		if err := s.Believe(&Belief{
			ID:         id,
			Content:    fmt.Sprintf("Belief %d derived from %s.", i, src),
			Confidence: 0.90 - float64(i)*0.01,
			Frame:      "bench",
			AssertedAt: now,
			Derivation: []string{src},
		}); err != nil {
			b.Fatalf("Believe: %v", err)
		}
	}
	return s, now
}

// buildWideStore creates a store with n beliefs all deriving from a single record.
// This is the shallow-and-wide pattern (fan-in on record).
func buildWideStore(b *testing.B, n int) (*Store, time.Time) {
	b.Helper()
	s := NewStore()
	s.RegisterFrame(Frame{Name: "bench", Decay: DecayPolicy{Kind: DecayNone}})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := s.Assert(&Record{ID: "r0", Content: "Shared base.", Frame: "bench", Timestamp: now}); err != nil {
		b.Fatalf("Assert: %v", err)
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("b%d", i)
		if err := s.Believe(&Belief{
			ID:         id,
			Content:    fmt.Sprintf("Parallel belief %d.", i),
			Confidence: 0.80,
			Frame:      "bench",
			AssertedAt: now,
			Derivation: []string{"r0"},
		}); err != nil {
			b.Fatalf("Believe: %v", err)
		}
	}
	return s, now
}

func BenchmarkQuery(b *testing.B) {
	s, now := buildBenchStore(b, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.Query("b49", now)
		if err != nil {
			b.Fatalf("Query: %v", err)
		}
	}
}

func BenchmarkProvenanceChain_Depth10(b *testing.B) {
	s, now := buildBenchStore(b, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.ProvenanceChain("b9", now)
		if err != nil {
			b.Fatalf("ProvenanceChain: %v", err)
		}
	}
}

func BenchmarkProvenanceChain_Depth50(b *testing.B) {
	s, now := buildBenchStore(b, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.ProvenanceChain("b49", now)
		if err != nil {
			b.Fatalf("ProvenanceChain: %v", err)
		}
	}
}

func BenchmarkAllBeliefs_100(b *testing.B) {
	s, now := buildWideStore(b, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.AllBeliefs(now)
	}
}

func BenchmarkQueryBeliefs(b *testing.B) {
	s, now := buildWideStore(b, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.QueryBeliefs("confidence >= 0.7 AND frame = bench", now)
		if err != nil {
			b.Fatalf("QueryBeliefs: %v", err)
		}
	}
}

func BenchmarkMinimalContraction(b *testing.B) {
	// Wide store: retracting r0 requires deciding whether to remove all 100 beliefs.
	// They all derive solely from r0, so they should all be removed.
	s, now := buildWideStore(b, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.MinimalContraction("r0", now)
		if err != nil {
			b.Fatalf("MinimalContraction: %v", err)
		}
	}
}

func BenchmarkBeliefHealth(b *testing.B) {
	s, now := buildBenchStore(b, 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.BeliefHealth("b19", now)
		if err != nil {
			b.Fatalf("BeliefHealth: %v", err)
		}
	}
}

func BenchmarkBuildSearchIndex(b *testing.B) {
	s, _ := buildWideStore(b, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.BuildSearchIndex()
	}
}

func BenchmarkExplain(b *testing.B) {
	s, now := buildBenchStore(b, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.Explain("b9", now)
		if err != nil {
			b.Fatalf("Explain: %v", err)
		}
	}
}
