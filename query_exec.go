package lumen

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// QueryEventKind identifies the category of an archaeological event.
type QueryEventKind string

const (
	QueryEventConfidenceChange QueryEventKind = "confidence-change"
	QueryEventSourceAdded      QueryEventKind = "source-added"
	QueryEventSourceRemoved    QueryEventKind = "source-removed"
	QueryEventRetraction       QueryEventKind = "retraction"
)

// QueryEvent is a single event in an epistemic archaeology timeline.
type QueryEvent struct {
	Kind     QueryEventKind
	At       time.Time
	BeliefID string

	// Confidence-change fields.
	ConfFrom float64
	ConfTo   float64
	Delta    float64 // ConfTo - ConfFrom
	Reason   string  // from VersionStore

	// Source-change fields.
	SourceID string
	Action   string // "added" or "removed"

	// Retraction fields.
	RecordID      string
	RetractReason string
}

// ArchiveResult holds the event timeline produced by executing a ParsedQuery.
// Named ArchiveResult to distinguish it from QueryResult (the predicate query type
// in query_lang.go / store.go) — these are different query systems.
type ArchiveResult struct {
	QueryID    string
	BeliefID   string
	Select     string
	Events     []QueryEvent
	ExecutedAt time.Time
	// Hint is set when the result is empty but there is likely a reason;
	// it suggests follow-up actions.
	Hint string
}

// ExecuteQuery runs a ParsedQuery against the store and returns a chronological
// event timeline. Supports three select kinds:
//   - "confidence-changes": one event per recorded confidence transition.
//   - "source-changes":     one event per source addition or removal.
//   - "retraction-events":  one event per retracted record in the provenance chain.
func (s *Store) ExecuteQuery(q ParsedQuery, now time.Time) (*ArchiveResult, error) {
	result := &ArchiveResult{
		QueryID:    q.ID,
		BeliefID:   q.Target,
		Select:     q.Select,
		ExecutedAt: now,
	}

	// Parse since timestamp. An unparseable value is an error, not a
	// silently skipped filter.
	var since time.Time
	if q.Since != "" {
		parsed := false
		for _, layout := range []string{time.RFC3339, "2006-01-02"} {
			if t, err := time.Parse(layout, q.Since); err == nil {
				since = t
				parsed = true
				break
			}
		}
		if !parsed {
			return nil, fmt.Errorf("query %s: since %q is not RFC3339 or YYYY-MM-DD", q.ID, q.Since)
		}
	}

	// Parse where predicate.
	pred, err := parseEventPredicate(q.Where)
	if err != nil {
		return nil, fmt.Errorf("query %s: where clause: %w", q.ID, err)
	}

	// Collect raw events.
	var rawEvents []QueryEvent
	switch q.Select {
	case "confidence-changes":
		rawEvents, err = s.execConfidenceChanges(q.Target, now)
		if err == nil && len(rawEvents) == 0 {
			result.Hint = "No explicit revisions recorded. The belief was asserted but never revised. " +
				"If the frame has decay, use 'bio " + q.Target + "' to see the decay trajectory."
		}
	case "source-changes":
		rawEvents, err = s.execSourceChanges(q.Target, now)
	case "retraction-events":
		rawEvents, err = s.execRetractionEvents(q.Target, now)
	default:
		return nil, fmt.Errorf("query %s: unknown select %q (want confidence-changes, source-changes, retraction-events)", q.ID, q.Select)
	}
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", q.ID, err)
	}

	// Filter.
	for _, ev := range rawEvents {
		if !since.IsZero() && ev.At.Before(since) {
			continue
		}
		if pred != nil && !pred.match(ev) {
			continue
		}
		result.Events = append(result.Events, ev)
	}

	return result, nil
}

// execConfidenceChanges emits one event per confidence transition in the version
// history. The version history captures the state *before* each mutation, so
// consecutive snapshots bracket each transition.
func (s *Store) execConfidenceChanges(beliefID string, now time.Time) ([]QueryEvent, error) {
	s.mu.RLock()
	b, ok := s.beliefs[beliefID]
	if !ok {
		s.mu.RUnlock()
		return nil, fmt.Errorf("belief %q not found", beliefID)
	}
	currentConf := b.Confidence
	s.mu.RUnlock()

	history := s.versions.History(beliefID)
	if len(history) == 0 {
		return nil, nil
	}

	var events []QueryEvent
	for i, v := range history {
		var toConf float64
		var reason string
		if i+1 < len(history) {
			toConf = history[i+1].Confidence
		} else {
			toConf = currentConf
		}
		reason = v.ChangeReason

		delta := toConf - v.Confidence
		if math.Abs(delta) < 1e-9 {
			continue
		}

		events = append(events, QueryEvent{
			Kind:     QueryEventConfidenceChange,
			At:       v.ChangedAt,
			BeliefID: beliefID,
			ConfFrom: v.Confidence,
			ConfTo:   toConf,
			Delta:    delta,
			Reason:   reason,
		})
	}
	return events, nil
}

// execSourceChanges emits one event per source ID added or removed between
// consecutive snapshots in the version history.
func (s *Store) execSourceChanges(beliefID string, now time.Time) ([]QueryEvent, error) {
	s.mu.RLock()
	b, ok := s.beliefs[beliefID]
	if !ok {
		s.mu.RUnlock()
		return nil, fmt.Errorf("belief %q not found", beliefID)
	}
	currentDeriv := make([]string, len(b.Derivation))
	copy(currentDeriv, b.Derivation)
	s.mu.RUnlock()

	history := s.versions.History(beliefID)
	if len(history) == 0 {
		return nil, nil
	}

	toSet := func(ids []string) map[string]struct{} {
		m := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			m[id] = struct{}{}
		}
		return m
	}

	var events []QueryEvent
	for i, v := range history {
		var nextDeriv []string
		var eventAt time.Time
		if i+1 < len(history) {
			nextDeriv = history[i+1].Derivation
		} else {
			nextDeriv = currentDeriv
		}
		eventAt = v.ChangedAt

		prev := toSet(v.Derivation)
		next := toSet(nextDeriv)

		for id := range next {
			if _, had := prev[id]; !had {
				events = append(events, QueryEvent{
					Kind:     QueryEventSourceAdded,
					At:       eventAt,
					BeliefID: beliefID,
					SourceID: id,
					Action:   "added",
				})
			}
		}
		for id := range prev {
			if _, has := next[id]; !has {
				events = append(events, QueryEvent{
					Kind:     QueryEventSourceRemoved,
					At:       eventAt,
					BeliefID: beliefID,
					SourceID: id,
					Action:   "removed",
				})
			}
		}
	}
	return events, nil
}

// execRetractionEvents walks the provenance chain and emits one event per
// retracted record in the ancestry.
func (s *Store) execRetractionEvents(beliefID string, now time.Time) ([]QueryEvent, error) {
	chain, err := s.ProvenanceChain(beliefID, now)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var events []QueryEvent
	for id, node := range chain.Nodes {
		if node.Kind != "record" {
			continue
		}
		rec, ok := s.records[id]
		if !ok || !rec.Retracted {
			continue
		}
		events = append(events, QueryEvent{
			Kind:          QueryEventRetraction,
			At:            rec.RetractedAt,
			BeliefID:      beliefID,
			RecordID:      rec.ID,
			RetractReason: rec.RetractReason,
		})
	}

	// Sort by retraction time ascending.
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].At.Before(events[j-1].At); j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}
	return events, nil
}

// --- Where-clause predicate ---

type eventPredicate struct {
	field string
	op    string
	value string
}

// parseEventPredicate parses a simple "field op value" where clause.
// Fields: change (float delta), source, action, reason, record.
// Ops: > < >= <= = == != contains startswith
func parseEventPredicate(where string) (*eventPredicate, error) {
	where = strings.TrimSpace(where)
	if where == "" {
		return nil, nil
	}
	tokens := tokeniseWhere(where)
	if len(tokens) < 3 {
		return nil, fmt.Errorf("expected 'field op value', got %q", where)
	}
	op := tokens[1]
	value := strings.Join(tokens[2:], " ")
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	switch op {
	case ">", "<", ">=", "<=", "=", "==", "!=", "contains", "startswith":
	default:
		return nil, fmt.Errorf("unknown operator %q (want >, <, >=, <=, =, !=, contains, startswith)", op)
	}
	return &eventPredicate{field: tokens[0], op: op, value: value}, nil
}

func tokeniseWhere(s string) []string {
	var tokens []string
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if unicode.IsSpace(runes[i]) {
			i++
			continue
		}
		if runes[i] == '"' {
			j := i + 1
			for j < len(runes) && runes[j] != '"' {
				j++
			}
			if j < len(runes) {
				j++
			}
			tokens = append(tokens, string(runes[i:j]))
			i = j
			continue
		}
		// Two-char operators.
		if i+1 < len(runes) {
			two := string(runes[i : i+2])
			if two == ">=" || two == "<=" || two == "!=" {
				tokens = append(tokens, two)
				i += 2
				continue
			}
		}
		if runes[i] == '>' || runes[i] == '<' || runes[i] == '=' {
			tokens = append(tokens, string(runes[i]))
			i++
			continue
		}
		j := i
		for j < len(runes) && !unicode.IsSpace(runes[j]) && runes[j] != '"' {
			j++
		}
		tokens = append(tokens, string(runes[i:j]))
		i = j
	}
	return tokens
}

func (p *eventPredicate) match(ev QueryEvent) bool {
	switch p.field {
	case "change":
		fv, err := strconv.ParseFloat(p.value, 64)
		if err != nil {
			return false
		}
		return cmpFloat(ev.Delta, p.op, fv)
	case "source":
		return cmpStr(ev.SourceID, p.op, p.value)
	case "action":
		return cmpStr(ev.Action, p.op, p.value)
	case "reason":
		return cmpStr(ev.Reason+ev.RetractReason, p.op, p.value)
	case "record":
		return cmpStr(ev.RecordID, p.op, p.value)
	}
	return true
}

func cmpFloat(actual float64, op string, threshold float64) bool {
	switch op {
	case ">":
		return actual > threshold
	case "<":
		return actual < threshold
	case ">=":
		return actual >= threshold
	case "<=":
		return actual <= threshold
	case "=", "==":
		return math.Abs(actual-threshold) < 1e-9
	case "!=":
		return math.Abs(actual-threshold) >= 1e-9
	}
	return false
}

func cmpStr(actual, op, value string) bool {
	switch op {
	case "=", "==":
		return actual == value
	case "!=":
		return actual != value
	case "contains":
		return strings.Contains(strings.ToLower(actual), strings.ToLower(value))
	case "startswith":
		return strings.HasPrefix(strings.ToLower(actual), strings.ToLower(value))
	}
	return false
}

// --- Rendering ---

// RenderArchiveResult returns a human-readable chronological timeline.
func RenderArchiveResult(r *ArchiveResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "query %s  ·  %s  ·  %s\n",
		r.QueryID, r.BeliefID, r.Select)
	fmt.Fprintf(&b, "executed %s  ·  %d event(s)\n\n",
		r.ExecutedAt.Format("2006-01-02 15:04:05"), len(r.Events))

	if len(r.Events) == 0 {
		fmt.Fprintln(&b, "  (no matching events)")
		if r.Hint != "" {
			fmt.Fprintf(&b, "  hint: %s\n", r.Hint)
		}
		return b.String()
	}

	for _, ev := range r.Events {
		fmt.Fprintf(&b, "  %s  %s\n", ev.At.Format("2006-01-02 15:04:05"), ev.Kind)
		switch ev.Kind {
		case QueryEventConfidenceChange:
			sign := "+"
			if ev.Delta < 0 {
				sign = ""
			}
			fmt.Fprintf(&b, "         %.0f%% → %.0f%%  (%s%.1f%%)",
				ev.ConfFrom*100, ev.ConfTo*100, sign, ev.Delta*100)
			if ev.Reason != "" && ev.Reason != "(current)" {
				fmt.Fprintf(&b, "  [%s]", ev.Reason)
			}
			fmt.Fprintln(&b)
		case QueryEventSourceAdded:
			fmt.Fprintf(&b, "         source %s  added\n", ev.SourceID)
		case QueryEventSourceRemoved:
			fmt.Fprintf(&b, "         source %s  removed\n", ev.SourceID)
		case QueryEventRetraction:
			fmt.Fprintf(&b, "         record %s retracted", ev.RecordID)
			if ev.RetractReason != "" {
				fmt.Fprintf(&b, "  reason: %q", ev.RetractReason)
			}
			fmt.Fprintln(&b)
		}
	}
	return b.String()
}
