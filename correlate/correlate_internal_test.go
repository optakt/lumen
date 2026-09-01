package correlate

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCosineToEpistemicCorrelation pins the calibrated piecewise transform so
// that a future threshold change cannot silently drift the documented mapping.
func TestCosineToEpistemicCorrelation(t *testing.T) {
	cases := []struct {
		cosine float64
		wantR  float64
	}{
		{0.00, 0.0},          // far below threshold
		{0.52, 0.0},          // just below weak root
		{0.53, 0.0},          // weak root lower edge
		{0.59, 0.2},          // (0.59-0.53)/0.12*0.40 = 0.20
		{0.65, 0.40},         // weak/moderate boundary
		{0.73, 0.5176470588}, // 0.40 + (0.73-0.65)/0.17*0.25
		{0.82, 0.65},         // moderate/strong boundary
		{0.91, 0.825},        // 0.65 + (0.91-0.82)/0.18*0.35
		{1.00, 1.0},          // clamped upper bound
	}
	for _, c := range cases {
		r, interp := cosineToEpistemicCorrelation(c.cosine)
		if math.Abs(r-c.wantR) > 1e-6 {
			t.Errorf("cosine %.2f: r = %.6f, want %.6f", c.cosine, r, c.wantR)
		}
		if interp == "" {
			t.Errorf("cosine %.2f: empty interpretation", c.cosine)
		}
	}
}

// TestCosineToEpistemicCorrelationMonotonic verifies the transform never
// decreases as cosine similarity increases across the full domain.
func TestCosineToEpistemicCorrelationMonotonic(t *testing.T) {
	prev := -1.0
	for c := 0.0; c <= 1.0001; c += 0.01 {
		r, _ := cosineToEpistemicCorrelation(c)
		if r < prev {
			t.Fatalf("non-monotonic at cosine=%.2f: r=%.6f < prev=%.6f", c, r, prev)
		}
		prev = r
	}
}

// TestEmbedOutOfRangeIndex verifies a malformed index returns an error rather
// than panicking.
func TestEmbedOutOfRangeIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"index": 5, "embedding": []float64{1, 2, 3}}},
		})
	}))
	defer server.Close()

	orig := voyageEndpoint
	voyageEndpoint = server.URL
	defer func() { voyageEndpoint = orig }()

	_, err := Embed("test-key", []string{"a", "b"})
	if err == nil {
		t.Fatal("expected out-of-range index error, got nil")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out-of-range error, got: %v", err)
	}
}

// TestEmbedMissingEmbedding verifies that an omitted embedding is reported
// rather than surfacing later as a confusing zero-vector or dimension error.
func TestEmbedMissingEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"index": 0, "embedding": []float64{1, 2, 3}}},
		})
	}))
	defer server.Close()

	orig := voyageEndpoint
	voyageEndpoint = server.URL
	defer func() { voyageEndpoint = orig }()

	_, err := Embed("test-key", []string{"a", "b"})
	if err == nil {
		t.Fatal("expected missing-embedding error, got nil")
	}
	if !strings.Contains(err.Error(), "missing embedding") {
		t.Fatalf("expected missing-embedding error, got: %v", err)
	}
}

// TestMostIsolatedSingleSource verifies a one-source matrix returns that source
// without dividing by zero.
func TestMostIsolatedSingleSource(t *testing.T) {
	m := &PairwiseMatrix{
		Sources: []StoreEvidence{{ID: "only", Content: "sole evidence"}},
		Correlations: [][]CorrelationResult{
			{{IDa: "only", IDb: "only", EstimatedCorrelation: 1.0}},
		},
	}
	got := m.MostIsolated()
	if got.ID != "only" {
		t.Fatalf("MostIsolated() = %q, want %q", got.ID, "only")
	}
}

// TestMostIsolatedPicksLowestAverage verifies the isolation metric selects the
// source with the lowest mean correlation to all others.
func TestMostIsolatedPicksLowestAverage(t *testing.T) {
	m := &PairwiseMatrix{
		Sources: []StoreEvidence{
			{ID: "a"}, {ID: "b"}, {ID: "c"},
		},
		Correlations: [][]CorrelationResult{
			// self=1.0 for each diagonal; off-diagonals below
			{{EstimatedCorrelation: 1.0}, {EstimatedCorrelation: 0.9}, {EstimatedCorrelation: 0.8}},
			{{EstimatedCorrelation: 0.9}, {EstimatedCorrelation: 1.0}, {EstimatedCorrelation: 0.7}},
			{{EstimatedCorrelation: 0.8}, {EstimatedCorrelation: 0.7}, {EstimatedCorrelation: 1.0}},
		},
	}
	got := m.MostIsolated()
	// a: (0.9+0.8)/2=0.85, b: (0.9+0.7)/2=0.80, c: (0.8+0.7)/2=0.75
	if got.ID != "c" {
		t.Fatalf("MostIsolated() = %q, want %q", got.ID, "c")
	}
}

// TestEmbedPreservesInputOrder verifies that Embed re-sorts out-of-order API
// indices back to input order.
func TestEmbedPreservesInputOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 1, "embedding": []float64{0, 1}},
				{"index": 0, "embedding": []float64{1, 0}},
			},
		})
	}))
	defer server.Close()

	orig := voyageEndpoint
	voyageEndpoint = server.URL
	defer func() { voyageEndpoint = orig }()

	got, err := Embed("test-key", []string{"first", "second"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(got))
	}
	if got[0][0] != 1 || got[1][0] != 0 {
		t.Fatalf("order not preserved: %v", got)
	}
}
