package lumen

import (
	"fmt"
	"sort"
	"time"
)

// AddQuery registers a named query in the store.
// Named queries are declared in .lm files and can be executed by ID.
// The store does not persist named queries to BoltDB — they are registered
// when the .lm file is loaded and must be re-loaded on restart.
func (s *Store) AddQuery(q ParsedQuery) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.namedQueries[q.ID] = q
}

// GetQuery returns a named query by ID.
func (s *Store) GetQuery(id string) (ParsedQuery, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q, ok := s.namedQueries[id]
	return q, ok
}

// AllQueries returns all registered named queries, sorted by ID.
func (s *Store) AllQueries() []ParsedQuery {
	s.mu.RLock()
	defer s.mu.RUnlock()
	qs := make([]ParsedQuery, 0, len(s.namedQueries))
	for _, q := range s.namedQueries {
		qs = append(qs, q)
	}
	sort.Slice(qs, func(i, j int) bool { return qs[i].ID < qs[j].ID })
	return qs
}

// RunQueryByID looks up a named query and executes it.
// Returns an error if the query ID is not found.
func (s *Store) RunQueryByID(id string, now time.Time) (*ArchiveResult, error) {
	q, ok := s.GetQuery(id)
	if !ok {
		// Provide a helpful list of available queries.
		all := s.AllQueries()
		ids := make([]string, len(all))
		for i, q := range all {
			ids[i] = q.ID
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("query %q not found (no queries registered — load a .lm file with query declarations)", id)
		}
		return nil, fmt.Errorf("query %q not found — available: %v", id, ids)
	}
	return s.ExecuteQuery(q, now)
}
