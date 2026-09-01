// Package correlate estimates pairwise evidence correlation using semantic embeddings.
// When two pieces of evidence share a common epistemic root — the same underlying
// intuition, the same dataset, the same author's framework — treating them as
// independent in Bayesian composition inflates the posterior. This package
// detects that overlap automatically by measuring embedding cosine similarity.
package correlate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

var voyageEndpoint = "https://api.voyageai.com/v1/embeddings"

// EvidencePair holds two evidence descriptions for correlation analysis.
type EvidencePair struct {
	IDa, IDb     string
	TextA, TextB string
}

// CorrelationResult is the estimated correlation between two pieces of evidence.
type CorrelationResult struct {
	IDa, IDb         string
	CosineSimilarity float64
	// EstimatedCorrelation maps cosine similarity to an epistemic correlation
	// coefficient using a calibrated piecewise transform (see
	// cosineToEpistemicCorrelation). Raw cosine similarity is not directly
	// usable as a Bayesian correlation: two semantically related but logically
	// independent claims (e.g. "fever" and "inflammation") would show high
	// similarity but low evidential correlation.
	EstimatedCorrelation float64
	Interpretation       string
}

type voyageRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type"`
}

type voyageResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// Embed returns a single embedding vector for the given text using Voyage AI.
func Embed(apiKey string, texts []string) ([][]float64, error) {
	body, _ := json.Marshal(voyageRequest{
		Input:     texts,
		Model:     "voyage-3",
		InputType: "document",
	})

	req, err := http.NewRequest("POST", voyageEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage request: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("voyage API %d: %s", resp.StatusCode, string(data))
	}

	var vr voyageResponse
	if err := json.Unmarshal(data, &vr); err != nil {
		return nil, fmt.Errorf("voyage parse: %w", err)
	}

	// Sort by index to preserve input order
	result := make([][]float64, len(texts))
	for _, item := range vr.Data {
		if item.Index < 0 || item.Index >= len(result) {
			return nil, fmt.Errorf("voyage response index %d out of range [0,%d)", item.Index, len(result))
		}
		result[item.Index] = item.Embedding
	}
	for i := range result {
		if result[i] == nil {
			return nil, fmt.Errorf("voyage response missing embedding for index %d", i)
		}
	}
	return result, nil
}

// CosineSimilarity computes cosine similarity between two L2-normalized vectors.
func CosineSimilarity(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("dimension mismatch: %d vs %d", len(a), len(b))
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0, fmt.Errorf("zero vector")
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB)), nil
}

// cosineToEpistemicCorrelation maps raw cosine similarity to an epistemic
// correlation coefficient. The mapping is:
//   - Below 0.53: treat as independent (r = 0)
//   - 0.53–0.65: weak shared root (r = 0.0–0.40)
//   - 0.65–0.82: moderate shared root (r = 0.40–0.65)
//   - 0.82–1.00: strong shared root (r = 0.65–1.0)
//
// These thresholds are calibrated on philosophical argument texts where:
// - Distinct arguments from the same tradition score ~0.65–0.75
// - Reformulations of the same argument score ~0.80–0.90
// - Near-identical claims score ~0.90+
func cosineToEpistemicCorrelation(cosine float64) (float64, string) {
	// Thresholds calibrated on Voyage-3 embeddings (2026-08-19):
	// zombie/knowledge cosine=0.594, cmd/zombie cosine=0.514, near-dup cosine=0.909
	switch {
	case cosine < 0.53:
		return 0, "independent — no meaningful shared epistemic root"
	case cosine < 0.65:
		r := (cosine - 0.53) / 0.12 * 0.40
		return r, fmt.Sprintf("weak shared root (r≈%.2f) — related tradition or surface vocabulary", r)
	case cosine < 0.82:
		r := 0.40 + (cosine-0.65)/0.17*0.25
		return r, fmt.Sprintf("moderate shared root (r≈%.2f) — same framework or underlying intuition", r)
	default:
		r := 0.65 + (cosine-0.82)/0.18*0.35
		if r > 1.0 {
			r = 1.0
		}
		return r, fmt.Sprintf("strong shared root (r≈%.2f) — near-reformulation of the same argument", r)
	}
}

// AnalyzePairs computes correlation estimates for all provided evidence pairs.
// A single API call is made with all unique texts batched together.
func AnalyzePairs(apiKey string, pairs []EvidencePair) ([]CorrelationResult, error) {
	// Collect unique texts
	textSet := make(map[string]int)
	var texts []string
	for _, p := range pairs {
		if _, ok := textSet[p.TextA]; !ok {
			textSet[p.TextA] = len(texts)
			texts = append(texts, p.TextA)
		}
		if _, ok := textSet[p.TextB]; !ok {
			textSet[p.TextB] = len(texts)
			texts = append(texts, p.TextB)
		}
	}

	embeddings, err := Embed(apiKey, texts)
	if err != nil {
		return nil, fmt.Errorf("embedding: %w", err)
	}

	results := make([]CorrelationResult, len(pairs))
	for i, p := range pairs {
		idxA := textSet[p.TextA]
		idxB := textSet[p.TextB]
		sim, err := CosineSimilarity(embeddings[idxA], embeddings[idxB])
		if err != nil {
			return nil, fmt.Errorf("pair %s/%s: %w", p.IDa, p.IDb, err)
		}
		r, interp := cosineToEpistemicCorrelation(sim)
		results[i] = CorrelationResult{
			IDa:                  p.IDa,
			IDb:                  p.IDb,
			CosineSimilarity:     sim,
			EstimatedCorrelation: r,
			Interpretation:       interp,
		}
	}
	return results, nil
}

// StoreEvidence is a text description of an evidence source in a belief store,
// used for automatic correlation analysis.
type StoreEvidence struct {
	ID      string
	Content string
}

// PairwiseMatrix computes all pairwise correlations for a set of evidence sources.
// Returns a symmetric matrix indexed by evidence ID.
type PairwiseMatrix struct {
	Sources []StoreEvidence
	// Correlations[i][j] = estimated correlation between Sources[i] and Sources[j]
	Correlations [][]CorrelationResult
}

// AnalyzePairwise computes all pairwise correlations for n evidence sources
// using a single batched embedding call. Time: O(n) API call, O(n²) computation.
func AnalyzePairwise(apiKey string, sources []StoreEvidence) (*PairwiseMatrix, error) {
	if len(sources) == 0 {
		return &PairwiseMatrix{}, nil
	}

	texts := make([]string, len(sources))
	for i, s := range sources {
		texts[i] = s.Content
	}

	embeddings, err := Embed(apiKey, texts)
	if err != nil {
		return nil, fmt.Errorf("embedding %d sources: %w", len(sources), err)
	}

	n := len(sources)
	matrix := make([][]CorrelationResult, n)
	for i := range matrix {
		matrix[i] = make([]CorrelationResult, n)
		for j := range matrix[i] {
			if i == j {
				matrix[i][j] = CorrelationResult{
					IDa: sources[i].ID, IDb: sources[i].ID,
					CosineSimilarity: 1.0, EstimatedCorrelation: 1.0,
					Interpretation: "self",
				}
				continue
			}
			sim, err := CosineSimilarity(embeddings[i], embeddings[j])
			if err != nil {
				return nil, fmt.Errorf("similarity %s/%s: %w", sources[i].ID, sources[j].ID, err)
			}
			r, interp := cosineToEpistemicCorrelation(sim)
			matrix[i][j] = CorrelationResult{
				IDa: sources[i].ID, IDb: sources[j].ID,
				CosineSimilarity: sim, EstimatedCorrelation: r,
				Interpretation: interp,
			}
		}
	}

	return &PairwiseMatrix{Sources: sources, Correlations: matrix}, nil
}

// HighCorrelationPairs returns all pairs with estimated correlation above threshold,
// sorted by correlation descending.
func (m *PairwiseMatrix) HighCorrelationPairs(threshold float64) []CorrelationResult {
	var pairs []CorrelationResult
	n := len(m.Sources)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			r := m.Correlations[i][j]
			if r.EstimatedCorrelation >= threshold {
				pairs = append(pairs, r)
			}
		}
	}
	// Sort by correlation descending
	for a := 0; a < len(pairs)-1; a++ {
		for b := a + 1; b < len(pairs); b++ {
			if pairs[b].EstimatedCorrelation > pairs[a].EstimatedCorrelation {
				pairs[a], pairs[b] = pairs[b], pairs[a]
			}
		}
	}
	return pairs
}

// MostIsolated returns the source whose average correlation with all others is lowest.
// An isolated source is one whose evidence is genuinely independent of the rest.
func (m *PairwiseMatrix) MostIsolated() StoreEvidence {
	if len(m.Sources) == 0 {
		return StoreEvidence{}
	}
	if len(m.Sources) == 1 {
		return m.Sources[0]
	}
	minAvg := math.MaxFloat64
	minIdx := 0
	n := len(m.Sources)
	for i := range m.Sources {
		sum := 0.0
		for j := range m.Sources {
			if i != j {
				sum += m.Correlations[i][j].EstimatedCorrelation
			}
		}
		avg := sum / float64(n-1)
		if avg < minAvg {
			minAvg = avg
			minIdx = i
		}
	}
	return m.Sources[minIdx]
}

// AverageCorrelation returns the mean estimated correlation across all pairs.
func (m *PairwiseMatrix) AverageCorrelation() float64 {
	n := len(m.Sources)
	if n < 2 {
		return 0
	}
	sum := 0.0
	count := 0
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			sum += m.Correlations[i][j].EstimatedCorrelation
			count++
		}
	}
	return sum / float64(count)
}
