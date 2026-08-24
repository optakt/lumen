package lumen

import (
	"fmt"
	"strings"
)

// ValidationIssue describes one consistency problem found in the store.
type ValidationIssue struct {
	Kind     string // "error" or "warning"
	BeliefID string // empty if not belief-specific
	RecordID string // empty if not record-specific
	Message  string
}

func (v ValidationIssue) String() string {
	target := ""
	if v.BeliefID != "" { target = "belief " + v.BeliefID + ": " }
	if v.RecordID != "" { target = "record " + v.RecordID + ": " }
	return fmt.Sprintf("[%s] %s%s", strings.ToUpper(v.Kind), target, v.Message)
}

// Validate checks the store for consistency issues.
//
// Checks performed:
//   1. Orphaned derivation references (belief derives from non-existent source)
//   2. Undefined frame references (belief or record names unknown frame)
//   3. Circular derivation (A derives from B derives from A)
//   4. Bridge frame references (bridge names unknown frames)
//   5. Belief confidence out of range [0, 1]
//   6. Empty content
func (s *Store) Validate() []ValidationIssue {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var issues []ValidationIssue
	warn := func(kind, beliefID, recordID, msg string) {
		issues = append(issues, ValidationIssue{Kind: kind, BeliefID: beliefID, RecordID: recordID, Message: msg})
	}

	// 1. Check belief derivation references.
	for id, b := range s.beliefs {
		for _, dep := range b.Derivation {
			if _, ok := s.records[dep]; ok { continue }
			if _, ok := s.beliefs[dep]; ok { continue }
			warn("error", id, "", fmt.Sprintf("derives from %q which does not exist", dep))
		}
		// 2. Undefined frame.
		if _, ok := s.frames[b.Frame]; !ok {
			warn("error", id, "", fmt.Sprintf("references unknown frame %q", b.Frame))
		}
		// 5. Confidence range.
		if b.Confidence < 0 || b.Confidence > 1 {
			warn("error", id, "", fmt.Sprintf("confidence %.3f out of range [0, 1]", b.Confidence))
		}
		// 6. Empty content.
		if strings.TrimSpace(b.Content) == "" {
			warn("warning", id, "", "empty content")
		}
	}

	// 2. Check record frame references.
	for id, r := range s.records {
		if _, ok := s.frames[r.Frame]; !ok {
			warn("error", "", id, fmt.Sprintf("references unknown frame %q", r.Frame))
		}
		if strings.TrimSpace(r.Content) == "" {
			warn("warning", "", id, "empty content")
		}
	}

	// 3. Circular derivation detection (DFS with path tracking).
	type state int
	const (
		unvisited state = iota
		inProgress
		done
	)
	visited := make(map[string]state)
	var dfs func(id string, path []string) bool
	dfs = func(id string, path []string) bool {
		switch visited[id] {
		case done:
			return false
		case inProgress:
			// Found a cycle. Identify the cycle.
			for i, p := range path {
				if p == id {
					cycle := append(path[i:], id)
					warn("error", id, "", fmt.Sprintf("circular derivation detected: %s", strings.Join(cycle, " → ")))
					return true
				}
			}
			return true
		}
		visited[id] = inProgress
		if b, ok := s.beliefs[id]; ok {
			for _, dep := range b.Derivation {
				dfs(dep, append(path, id))
			}
		}
		visited[id] = done
		return false
	}
	for id := range s.beliefs {
		if visited[id] == unvisited {
			dfs(id, nil)
		}
	}

	// 4. Bridge frame references.
	for _, br := range s.Bridges.bridges {
		if _, ok := s.frames[br.FromFrame]; !ok {
			warn("warning", "", "", fmt.Sprintf("bridge %q: from-frame %q not defined", br.Name, br.FromFrame))
		}
		if _, ok := s.frames[br.ToFrame]; !ok {
			warn("warning", "", "", fmt.Sprintf("bridge %q: to-frame %q not defined", br.Name, br.ToFrame))
		}
	}

	return issues
}
