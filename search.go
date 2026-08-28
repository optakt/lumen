package lumen

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
)

// SearchIndex is a TF-IDF index over belief and record content.
// It enables semantic similarity search without external dependencies.
//
// TF-IDF (term frequency-inverse document frequency) scores words by:
//   - TF: how often a term appears in this document
//   - IDF: how rare the term is across all documents
//
// Cosine similarity between two documents' TF-IDF vectors gives a
// semantic similarity score that captures "about the same thing"
// better than exact string matching.
//
// Vectors and per-document norms are pre-computed at index build time so
// that each query only performs n dot products — no per-query map allocation.
type SearchIndex struct {
	// docs maps node ID → raw term frequency map (kept for IDF reuse)
	docs map[string]map[string]float64
	// vecs maps node ID → pre-computed TF-IDF vector
	vecs map[string]map[string]float64
	// norms maps node ID → pre-computed L2 norm of its TF-IDF vector
	norms map[string]float64
	// idf maps term → log(N / df) where df is document frequency
	idf map[string]float64
	// N is total document count
	N int
}

// SearchResult is one result from a search query.
type SearchResult struct {
	NodeID     string
	Kind       string // "belief" or "record"
	Content    string
	Similarity float64
}

// BuildSearchIndex creates a TF-IDF index from the current store state.
// Call this after loading or updating the store; it's not maintained incrementally.
func (s *Store) BuildSearchIndex() *SearchIndex {
	s.mu.RLock()
	defer s.mu.RUnlock()

	idx := &SearchIndex{
		docs: make(map[string]map[string]float64),
		idf:  make(map[string]float64),
	}

	// Count document frequencies for IDF
	df := make(map[string]int)

	// Index records
	for id, rec := range s.records {
		terms := tokenize(rec.Content)
		tf := termFreq(terms)
		idx.docs[id] = tf
		for term := range tf {
			df[term]++
		}
	}

	// Index beliefs (skip contracted/superseded — they are soft-deleted).
	for id, b := range s.beliefs {
		if b.State == BeliefSuperseded {
			continue
		}
		terms := tokenize(b.Content)
		tf := termFreq(terms)
		idx.docs[id] = tf
		for term := range tf {
			df[term]++
		}
	}

	idx.N = len(idx.docs)

	// Compute IDF
	for term, count := range df {
		idx.idf[term] = math.Log(float64(idx.N+1) / float64(count+1))
	}

	// Pre-compute TF-IDF vectors and their L2 norms.
	// This trades index build time for O(1) query setup: instead of
	// recomputing all document vectors on every query, each search only needs
	// to compute the query vector and then dot it against pre-built doc vectors.
	idx.vecs = make(map[string]map[string]float64, len(idx.docs))
	idx.norms = make(map[string]float64, len(idx.docs))
	for id, tf := range idx.docs {
		vec := tfidfVector(tf, idx.idf)
		idx.vecs[id] = vec
		idx.norms[id] = vecNorm(vec)
	}

	return idx
}

// Search returns the top-k most similar nodes to the query string.
// Results are sorted by similarity descending.
// Uses pre-computed per-document TF-IDF vectors and norms; the query is the
// only computation that happens at search time.
func (s *Store) Search(idx *SearchIndex, query string, topK int) []SearchResult {
	if idx == nil || len(idx.docs) == 0 {
		return nil
	}

	queryTerms := tokenize(query)
	queryTF := termFreq(queryTerms)
	queryVec := tfidfVector(queryTF, idx.idf)
	queryNorm := vecNorm(queryVec)
	if queryNorm == 0 {
		return nil // query contains no indexed terms
	}

	type scored struct {
		id    string
		score float64
	}
	var candidates []scored

	for id, docVec := range idx.vecs {
		// Dot product of query with pre-computed document vector.
		dot := 0.0
		for term, qv := range queryVec {
			if dv, ok := docVec[term]; ok {
				dot += qv * dv
			}
		}
		docNorm := idx.norms[id]
		if docNorm == 0 {
			continue
		}
		sim := dot / (queryNorm * docNorm)
		if sim > 0.01 {
			candidates = append(candidates, scored{id, sim})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]SearchResult, 0, len(candidates))
	for _, c := range candidates {
		var kind, content string
		if rec, ok := s.records[c.id]; ok {
			kind = "record"
			content = rec.Content
		} else if b, ok := s.beliefs[c.id]; ok {
			kind = "belief"
			content = b.Content
		} else {
			continue
		}
		results = append(results, SearchResult{
			NodeID:     c.id,
			Kind:       kind,
			Content:    content,
			Similarity: c.score,
		})
	}
	return results
}

// SimilarBeliefs returns beliefs similar to a given belief, using TF-IDF.
func (s *Store) SimilarBeliefs(idx *SearchIndex, beliefID string, topK int) ([]SearchResult, error) {
	s.mu.RLock()
	b, ok := s.beliefs[beliefID]
	if !ok {
		s.mu.RUnlock()
		return nil, fmt.Errorf("belief %s not found", beliefID)
	}
	content := b.Content
	s.mu.RUnlock()

	results := s.Search(idx, content, topK+1) // +1 because the belief itself will match

	// Remove the query belief from results
	filtered := results[:0]
	for _, r := range results {
		if r.NodeID != beliefID {
			filtered = append(filtered, r)
		}
	}
	if topK > 0 && len(filtered) > topK {
		filtered = filtered[:topK]
	}
	return filtered, nil
}

// helpers

// stopWords are common English words excluded from indexing.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
	"in": true, "of": true, "to": true, "for": true, "by": true, "at": true,
	"on": true, "as": true, "it": true, "its": true, "that": true, "this": true,
	"with": true, "from": true, "has": true, "have": true, "had": true,
	"not": true, "no": true, "does": true, "do": true, "did": true,
	"which": true, "who": true, "what": true, "when": true, "where": true,
	"how": true, "if": true, "then": true, "so": true, "also": true,
	"may": true, "can": true, "will": true, "would": true, "could": true, "should": true,
}

func tokenize(text string) []string {
	lower := strings.ToLower(text)
	var tokens []string
	current := strings.Builder{}
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 2 { // ignore very short tokens
				word := current.String()
				if !stopWords[word] {
					tokens = append(tokens, word)
				}
			}
			current.Reset()
		}
	}
	if current.Len() > 2 {
		word := current.String()
		if !stopWords[word] {
			tokens = append(tokens, word)
		}
	}
	return tokens
}

func termFreq(tokens []string) map[string]float64 {
	tf := make(map[string]float64)
	for _, t := range tokens {
		tf[t]++
	}
	// Normalize by document length
	if len(tokens) > 0 {
		for k := range tf {
			tf[k] /= float64(len(tokens))
		}
	}
	return tf
}

func tfidfVector(tf, idf map[string]float64) map[string]float64 {
	vec := make(map[string]float64, len(tf))
	for term, freq := range tf {
		if score, ok := idf[term]; ok {
			vec[term] = freq * score
		}
	}
	return vec
}

func vecNorm(v map[string]float64) float64 {
	sum := 0.0
	for _, val := range v {
		sum += val * val
	}
	return math.Sqrt(sum)
}

// CachedSearch returns the top-k results for query using a lazily rebuilt
// search index. The index is rebuilt on the first call after any write;
// subsequent reads return instantly from the cache.
//
// This replaces the pattern of calling BuildSearchIndex() + Search() on every
// HTTP request, which scales as O(n) per request and dominates latency at 10k+
// beliefs.
func (s *Store) CachedSearch(query string, topK int) []SearchResult {
	s.mu.RLock()
	if !s.searchDirty && s.searchIndex != nil {
		idx := s.searchIndex
		s.mu.RUnlock()
		return s.Search(idx, query, topK)
	}
	generation := s.searchGeneration
	s.mu.RUnlock()

	// Rebuild outside the read lock (BuildSearchIndex takes its own read lock).
	idx := s.BuildSearchIndex()

	s.mu.Lock()
	if s.searchGeneration == generation {
		s.searchIndex = idx
		s.searchDirty = false
	}
	s.mu.Unlock()

	return s.Search(idx, query, topK)
}

// invalidateSearch marks the search index cache as dirty.
// Must be called under s.mu.Lock() by any write that adds or removes indexable content.
func (s *Store) invalidateSearch() {
	s.searchDirty = true
	s.searchGeneration++
}
