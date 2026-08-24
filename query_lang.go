package lumen

// Package-level query language for the Lumen belief store.
//
// Grammar:
//   query     = expr
//   expr      = term (("AND" | "OR") term)*
//   term      = "NOT" term | "(" expr ")" | predicate
//   predicate = field op value
//   field     = "confidence" | "frame" | "content" | "state" | "id" | "kind"
//   op        = "=" | "!=" | ">" | "<" | ">=" | "<=" | "contains" | "startswith"
//   value     = quoted_string | number | bare_word
//
// Examples:
//   confidence > 0.7
//   frame = philosophical AND confidence > 0.5
//   content contains "consciousness" AND state = active
//   NOT state = suspect
//   (frame = empirical OR frame = contemporary) AND confidence >= 0.8
//
// Fields:
//   confidence — current computed confidence (float, 0–1)
//   frame      — frame name (string)
//   content    — belief content text (string, supports contains/startswith)
//   state      — active | suspect | stale
//   id         — belief ID (string)
//   kind       — claim kind if extracted via analyze (string)

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// QueryResult holds the result of a belief store query.
type QueryMatch struct {
	BeliefID          string
	Content           string
	Frame             string
	CurrentConfidence float64
	State             BeliefState
	AssertedAt        time.Time
}

// QueryBeliefs evaluates a query string against the store and returns matching beliefs.
func (s *Store) QueryBeliefs(query string, now time.Time) ([]QueryMatch, error) {
	pred, err := parseQuery(query)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []QueryMatch
	for _, b := range s.beliefs {
		// Skip contracted beliefs — they are soft-deleted.
		// Use ContractedBeliefs() to inspect them directly.
		if b.State == BeliefSuperseded {
			continue
		}
		frame := s.frames[b.Frame]
		conf := b.CurrentConfidence(frame, now)
		if pred.eval(b, conf) {
			results = append(results, QueryMatch{
				BeliefID:          b.ID,
				Content:           b.Content,
				Frame:             b.Frame,
				CurrentConfidence: conf,
				State:             b.State,
				AssertedAt:        b.AssertedAt,
			})
		}
	}
	return results, nil
}

// ── AST ──────────────────────────────────────────────────────────────────────

type predNode interface {
	eval(b *Belief, conf float64) bool
}

type andNode struct{ left, right predNode }
type orNode  struct{ left, right predNode }
type notNode struct{ child predNode }

func (n *andNode) eval(b *Belief, c float64) bool { return n.left.eval(b, c) && n.right.eval(b, c) }
func (n *orNode)  eval(b *Belief, c float64) bool { return n.left.eval(b, c) || n.right.eval(b, c) }
func (n *notNode) eval(b *Belief, c float64) bool { return !n.child.eval(b, c) }

type predicateNode struct {
	field string
	op    string
	value string
	num   float64
	isNum bool
}

func (p *predicateNode) eval(b *Belief, conf float64) bool {
	switch p.field {
	case "confidence":
		return compareNum(conf, p.op, p.num)
	case "frame":
		return compareStr(b.Frame, p.op, p.value)
	case "content":
		return compareStr(strings.ToLower(b.Content), p.op, strings.ToLower(p.value))
	case "state":
		stateStr := stateToString(b.State)
		return compareStr(stateStr, p.op, strings.ToLower(p.value))
	case "id":
		return compareStr(b.ID, p.op, p.value)
	}
	return false
}

func stateToString(s BeliefState) string {
	switch s {
	case BeliefActive:     return "active"
	case BeliefSuspect:    return "suspect"
	case BeliefStale:      return "stale"
	case BeliefSuperseded: return "superseded"
	default:               return "unknown"
	}
}

func compareNum(a float64, op string, b float64) bool {
	switch op {
	case "=", "==": return math.Abs(a-b) < 0.0001
	case "!=":       return math.Abs(a-b) >= 0.0001
	case ">":        return a > b
	case "<":        return a < b
	case ">=":       return a >= b
	case "<=":       return a <= b
	}
	return false
}

func compareStr(a, op, b string) bool {
	switch op {
	case "=", "==":     return a == b
	case "!=":          return a != b
	case "contains":    return strings.Contains(a, b)
	case "startswith":  return strings.HasPrefix(a, b)
	case ">":           return a > b
	case "<":           return a < b
	}
	return false
}

// ── Parser ────────────────────────────────────────────────────────────────────

type qtoken struct {
	kind string // "word", "op", "lparen", "rparen", "string", "number"
	val  string
}

func tokenizeQuery(s string) []qtoken {
	var tokens []qtoken
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) {
		// Skip whitespace
		if unicode.IsSpace(rune(s[i])) {
			i++
			continue
		}
		// Quoted string
		if s[i] == '"' || s[i] == '\'' {
			q := s[i]
			i++
			start := i
			for i < len(s) && s[i] != q {
				i++
			}
			tokens = append(tokens, qtoken{"string", s[start:i]})
			if i < len(s) { i++ }
			continue
		}
		// Parens
		if s[i] == '(' {
			tokens = append(tokens, qtoken{"lparen", "("})
			i++
			continue
		}
		if s[i] == ')' {
			tokens = append(tokens, qtoken{"rparen", ")"})
			i++
			continue
		}
		// Two-char ops: >=, <=, !=
		if i+1 < len(s) {
			two := s[i : i+2]
			if two == ">=" || two == "<=" || two == "!=" || two == "==" {
				tokens = append(tokens, qtoken{"op", two})
				i += 2
				continue
			}
		}
		// Single-char ops
		if s[i] == '>' || s[i] == '<' || s[i] == '=' {
			tokens = append(tokens, qtoken{"op", string(s[i])})
			i++
			continue
		}
		// Number (including negative)
		if unicode.IsDigit(rune(s[i])) || (s[i] == '-' && i+1 < len(s) && unicode.IsDigit(rune(s[i+1]))) {
			start := i
			if s[i] == '-' { i++ }
			for i < len(s) && (unicode.IsDigit(rune(s[i])) || s[i] == '.') {
				i++
			}
			tokens = append(tokens, qtoken{"number", s[start:i]})
			continue
		}
		// Word (identifier or keyword)
		start := i
		for i < len(s) && !unicode.IsSpace(rune(s[i])) && s[i] != '(' && s[i] != ')' && s[i] != '"' && s[i] != '\'' {
			i++
		}
		if i > start {
			tokens = append(tokens, qtoken{"word", s[start:i]})
		}
	}
	return tokens
}

type qparser struct {
	tokens []qtoken
	pos    int
}

func (p *qparser) peek() *qtoken {
	if p.pos >= len(p.tokens) { return nil }
	return &p.tokens[p.pos]
}
func (p *qparser) next() *qtoken {
	if p.pos >= len(p.tokens) { return nil }
	t := &p.tokens[p.pos]
	p.pos++
	return t
}

func parseQuery(s string) (predNode, error) {
	tokens := tokenizeQuery(s)
	p := &qparser{tokens: tokens}
	node, err := p.parseExpr()
	if err != nil { return nil, err }
	if p.peek() != nil {
		return nil, fmt.Errorf("unexpected token %q", p.peek().val)
	}
	return node, nil
}

func (p *qparser) parseExpr() (predNode, error) {
	left, err := p.parseTerm()
	if err != nil { return nil, err }

	for {
		t := p.peek()
		if t == nil { break }
		if t.kind != "word" { break }
		upper := strings.ToUpper(t.val)
		if upper != "AND" && upper != "OR" { break }
		p.next()
		right, err := p.parseTerm()
		if err != nil { return nil, err }
		if upper == "AND" {
			left = &andNode{left, right}
		} else {
			left = &orNode{left, right}
		}
	}
	return left, nil
}

func (p *qparser) parseTerm() (predNode, error) {
	t := p.peek()
	if t == nil { return nil, fmt.Errorf("unexpected end of query") }

	if t.kind == "word" && strings.ToUpper(t.val) == "NOT" {
		p.next()
		child, err := p.parseTerm()
		if err != nil { return nil, err }
		return &notNode{child}, nil
	}
	if t.kind == "lparen" {
		p.next()
		node, err := p.parseExpr()
		if err != nil { return nil, err }
		if close := p.next(); close == nil || close.kind != "rparen" {
			return nil, fmt.Errorf("expected closing ')'")
		}
		return node, nil
	}
	return p.parsePredicate()
}

func (p *qparser) parsePredicate() (predNode, error) {
	field := p.next()
	if field == nil || field.kind != "word" {
		return nil, fmt.Errorf("expected field name")
	}
	op := p.next()
	if op == nil {
		return nil, fmt.Errorf("expected operator after %q", field.val)
	}
	// Handle multi-word ops: "contains", "startswith"
	opStr := strings.ToLower(op.val)
	if op.kind != "op" && op.kind != "word" {
		return nil, fmt.Errorf("expected operator, got %q", op.val)
	}

	val := p.next()
	if val == nil {
		return nil, fmt.Errorf("expected value after operator %q", opStr)
	}

	fieldName := strings.ToLower(field.val)
	numericFields := map[string]bool{"confidence": true}
	stringFields  := map[string]bool{"frame": true, "content": true, "state": true, "id": true}

	if numericFields[fieldName] {
		if val.kind != "number" {
			return nil, fmt.Errorf("field %q requires a numeric value, got %q", fieldName, val.val)
		}
	} else if stringFields[fieldName] {
		if val.kind == "number" {
			return nil, fmt.Errorf("field %q requires a string value, got numeric %q", fieldName, val.val)
		}
	} else {
		return nil, fmt.Errorf("unknown field %q; valid: confidence, frame, content, state, id", fieldName)
	}

	pn := &predicateNode{
		field: fieldName,
		op:    opStr,
		value: val.val,
	}
	if val.kind == "number" {
		f, err := strconv.ParseFloat(val.val, 64)
		if err != nil { return nil, fmt.Errorf("invalid number %q", val.val) }
		pn.num = f
		pn.isNum = true
	}
	return pn, nil
}
