package lumen

import (
	"strings"
	"testing"
	"time"
)

const medicalProgram = `
// Lumen medical diagnosis example

frame medical
    composition: bayesian
    decay: exponential halflife: 30d
    provenance-depth: 3
    imported-decay: most_conservative

frame sensor
    composition: bayesian
    decay: exponential halflife: 1h
    imported-decay: most_conservative

record bp-001 in medical
    "Blood pressure 140/90 at 14:00"
    at: "2026-01-01T14:00:00Z"

record temp-001 in sensor
    "Body temperature 38.5C"
    at: "2026-01-01T14:00:00Z"

belief fever-001 in sensor
    "Sensor indicates elevated temperature"
    confidence: 0.93
    from: temp-001

belief hypertension-001 in medical
    "Patient likely has hypertension"
    confidence: 0.82
    from: bp-001

belief diagnosis-001 in medical
    "Hypertensive patient with fever, likely viral"
    confidence: 0.85
    from: hypertension-001, fever-001
`

func TestParseMedicalProgram(t *testing.T) {
	tokens, err := Tokenize(medicalProgram)
	if err != nil {
		t.Fatalf("lex error: %v", err)
	}

	// Log token stream (abbreviated)
	var kindSummary []string
	for _, tok := range tokens {
		if tok.Kind != TokNewline {
			kindSummary = append(kindSummary, tok.Kind.String())
		}
	}
	t.Logf("tokens: %s", strings.Join(kindSummary[:min(30, len(kindSummary))], " "))

	p := NewParser(tokens)
	file, err := p.Parse()
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(file.Frames) != 2 {
		t.Errorf("expected 2 frames, got %d", len(file.Frames))
	}
	if len(file.Records) != 2 {
		t.Errorf("expected 2 records, got %d: %+v", len(file.Records), file.Records)
	}
	if len(file.Beliefs) != 3 {
		t.Errorf("expected 3 beliefs, got %d: %+v", len(file.Beliefs), file.Beliefs)
	}

	// Check frame parsing
	for _, f := range file.Frames {
		t.Logf("frame %s: decay=%s halflife=%v", f.Name, f.Decay.Kind, f.Decay.Halflife)
	}

	// Check belief parsing
	diag := file.Beliefs[2]
	if diag.ID != "diagnosis-001" {
		t.Errorf("expected diagnosis-001, got %s", diag.ID)
	}
	if len(diag.From) != 2 {
		t.Errorf("expected 2 sources for diagnosis-001, got %d: %v", len(diag.From), diag.From)
	}
	if diag.Confidence != 0.85 {
		t.Errorf("expected confidence 0.85, got %f", diag.Confidence)
	}
}

func TestLoadFile(t *testing.T) {
	s := NewStore()
	now := time.Date(2026, 1, 1, 15, 0, 0, 0, time.UTC)
	if err := LoadFile(medicalProgram, s, now); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// Query each belief
	cases := []struct {
		id      string
		minConf float64
		maxConf float64
	}{
		{"fever-001", 0.92, 0.94},
		{"hypertension-001", 0.81, 0.83},
		{"diagnosis-001", 0.84, 0.86},
	}
	for _, tc := range cases {
		r, err := s.Query(tc.id, now)
		if err != nil {
			t.Errorf("Query %s: %v", tc.id, err)
			continue
		}
		if r.CurrentConfidence < tc.minConf || r.CurrentConfidence > tc.maxConf {
			t.Errorf("%s: got %.4f want [%.2f, %.2f]", tc.id, r.CurrentConfidence, tc.minConf, tc.maxConf)
		}
		t.Logf("%s: %.4f (%s)", tc.id, r.CurrentConfidence, r.Content)
	}
}

func TestLexerTokenTypes(t *testing.T) {
	cases := []struct {
		src  string
		want TokenKind
		val  string
	}{
		{`"hello world"`, TokString, "hello world"},
		{"30d", TokDuration, "30d"},
		{"1h", TokDuration, "1h"},
		{"0.85", TokFloat, "0.85"},
		{"bayesian", TokIdent, "bayesian"},
		{":", TokColon, ":"},
	}
	for _, tc := range cases {
		tokens, err := Tokenize(tc.src)
		if err != nil {
			t.Errorf("lex %q: %v", tc.src, err)
			continue
		}
		if len(tokens) == 0 || tokens[0].Kind != tc.want {
			t.Errorf("lex %q: got %v want %s", tc.src, tokens, tc.want)
		}
		if tokens[0].Value != tc.val {
			t.Errorf("lex %q value: got %q want %q", tc.src, tokens[0].Value, tc.val)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestParseCorrelationStatement(t *testing.T) {
	src := `
frame philosophical
    composition: bayesian
    decay: none
    provenance-depth: 3
    imported-decay: most_conservative

record zombie in philosophical
    "Philosophical zombie argument"
    at: "2026-01-01T00:00:00Z"

record knowledge in philosophical
    "Mary's Room knowledge argument"
    at: "2026-01-01T00:00:00Z"

correlation zombie knowledge: 0.70

retract zombie reason: "argument retracted for testing"
`
	result, err := ParseFull(src)
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}

	if len(result.Frames) != 1 {
		t.Errorf("expected 1 frame, got %d", len(result.Frames))
	}
	if len(result.Records) != 2 {
		t.Errorf("expected 2 records, got %d", len(result.Records))
	}
	if len(result.Correlations) != 1 {
		t.Errorf("expected 1 correlation, got %d", len(result.Correlations))
	} else {
		c := result.Correlations[0]
		if c.IDA != "zombie" || c.IDB != "knowledge" {
			t.Errorf("expected zombie/knowledge, got %s/%s", c.IDA, c.IDB)
		}
		if c.Correlation != 0.70 {
			t.Errorf("expected r=0.70, got %.2f", c.Correlation)
		}
	}
	if len(result.Retracts) != 1 {
		t.Errorf("expected 1 retract, got %d", len(result.Retracts))
	} else {
		r := result.Retracts[0]
		if r.ID != "zombie" {
			t.Errorf("expected retract zombie, got %s", r.ID)
		}
		if r.Reason != "argument retracted for testing" {
			t.Errorf("unexpected reason: %q", r.Reason)
		}
	}
	t.Logf("ParseFull: %d frames, %d records, %d correlations, %d retracts",
		len(result.Frames), len(result.Records), len(result.Correlations), len(result.Retracts))
}

func TestParseImport(t *testing.T) {
	src := `
import "base.lm"
import "medical.lm"

record r1 in default
    "Some record"
    at: "2026-01-01T00:00:00Z"
`
	// Register default frame to avoid errors during full load
	result, err := ParseFull(src)
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}

	if len(result.Imports) != 2 {
		t.Errorf("expected 2 imports, got %d", len(result.Imports))
	}
	if result.Imports[0].Path != "base.lm" {
		t.Errorf("expected base.lm, got %q", result.Imports[0].Path)
	}
	if result.Imports[1].Path != "medical.lm" {
		t.Errorf("expected medical.lm, got %q", result.Imports[1].Path)
	}
	t.Logf("imports: %v", result.Imports)
}

func TestParseRetractStandaloneWithoutReason(t *testing.T) {
	src := `
record r1 in default
    "A record"
    at: "2026-01-01T00:00:00Z"

retract r1
`
	result, err := ParseFull(src)
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	if len(result.Retracts) != 1 {
		t.Errorf("expected 1 retract, got %d", len(result.Retracts))
	}
	if result.Retracts[0].Reason != "" {
		t.Errorf("expected empty reason, got %q", result.Retracts[0].Reason)
	}
	t.Logf("retract without reason: id=%s reason=%q", result.Retracts[0].ID, result.Retracts[0].Reason)
}

func TestParserFuzzMalformed(t *testing.T) {
	// Fuzz-style test: malformed .lm inputs should return errors, never panic
	malformed := []string{
		// Empty
		"",
		// Just a comment
		"// hello",
		// Incomplete frame
		"frame",
		"frame medical",
		"frame medical\n    composition:",
		// Unknown keyword
		"blorp foo bar",
		// Record without frame
		"record r1\n    \"content\"\n    at: \"2026-01-01T00:00:00Z\"",
		// Believe without frame
		"believe b1\n    \"content\"\n    confidence: 0.5",
		// Negative confidence (no syntactic restriction, but semantically odd)
		"frame f\n    composition: bayesian\n    decay: none\n    provenance-depth: 1\n    imported-decay: most_conservative\nrecord r1 in f\n    \"test\"\n    at: \"2026-01-01T00:00:00Z\"\nbelieve b1 in f\n    \"belief\"\n    confidence: -0.5\n    from: r1",
		// Correlation with only one ID
		"correlation zombie",
		// Correlation with no colon
		"correlation zombie knowledge 0.5",
		// Import without path
		"import",
		// Deeply nested nonsense
		"frame a\n    composition: bayesian\n    decay: none\n    provenance-depth: 1\n    imported-decay: most_conservative\n" +
			"frame b\n    composition: bayesian\n    decay: none\n    provenance-depth: 1\n    imported-decay: most_conservative\n" +
			strings.Repeat("record rx in a\n    \"test\"\n    at: \"2026-01-01T00:00:00Z\"\n", 50),
		// Malformed duration
		"frame f\n    composition: bayesian\n    decay: exponential halflife: xyz\n    provenance-depth: 1\n    imported-decay: most_conservative",
		// Truncated mid-string
		"record r1 in medical\n    \"unterminated string",
	}

	panics := 0
	errors := 0
	successes := 0

	for i, src := range malformed {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("input %d panicked: %v\nsrc: %q", i, r, src[:min2(len(src), 80)])
					panics++
				}
			}()
			_, err := ParseFull(src)
			if err != nil {
				errors++
			} else {
				successes++
			}
		}()
	}
	t.Logf("Fuzz results: %d panics, %d errors, %d successes across %d inputs",
		panics, errors, successes, len(malformed))
	if panics > 0 {
		t.Errorf("%d panics encountered", panics)
	}
}

func min2(a, b int) int {
	if a < b { return a }
	return b
}

func TestCredalPriorParsing(t *testing.T) {
	src := `
frame epistemic
    composition: bayesian
    decay: none
    provenance-depth: 3
    imported-decay: most_conservative

record zombie-evidence in epistemic
    "Philosophical zombie argument for the hard problem"
    at: "2026-01-01T00:00:00Z"

believe hard-problem-real in epistemic
    "The hard problem of consciousness is a genuine explanatory gap"
    prior: [0.35, 0.65]
    from: zombie-evidence
`
	result, err := ParseFull(src)
	if err != nil {
		t.Fatalf("ParseFull: %v", err)
	}
	if len(result.Beliefs) != 1 {
		t.Fatalf("expected 1 belief, got %d", len(result.Beliefs))
	}
	b := result.Beliefs[0]
	if !b.HasCredalPrior {
		t.Error("expected HasCredalPrior=true")
	}
	if b.CredalPriorLo != 0.35 {
		t.Errorf("expected lo=0.35, got %f", b.CredalPriorLo)
	}
	if b.CredalPriorHi != 0.65 {
		t.Errorf("expected hi=0.65, got %f", b.CredalPriorHi)
	}
	t.Logf("Credal prior parsed: [%.2f, %.2f]", b.CredalPriorLo, b.CredalPriorHi)
}

func TestBeliefAtTimestamp(t *testing.T) {
	src := `
frame historical
    decay: none

record event-1900 in historical
    "Something happened in 1900."
    at: "1900-01-01"

believe conclusion-1910 in historical
    "A conclusion formed in 1910."
    confidence: 0.75
    at: "1910-06-15"
    from: event-1900
`
	s := NewStore()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(src, s, now); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	beliefs := s.AllBeliefs(now)
	if len(beliefs) != 1 {
		t.Fatalf("expected 1 belief, got %d", len(beliefs))
	}
	b := beliefs[0]
	if b.BeliefID != "conclusion-1910" {
		t.Fatalf("unexpected belief ID %q", b.BeliefID)
	}

	// Verify the assertion timestamp was parsed and set.
	snap1910 := s.SnapshotAt(time.Date(1910, 6, 15, 0, 0, 0, 0, time.UTC))
	if len(snap1910.AllBeliefs(time.Date(1910, 6, 15, 0, 0, 0, 0, time.UTC))) != 1 {
		t.Error("belief should appear in snapshot at its assertion date")
	}
	snap1909 := s.SnapshotAt(time.Date(1909, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(snap1909.AllBeliefs(time.Date(1909, 1, 1, 0, 0, 0, 0, time.UTC))) != 0 {
		t.Error("belief should not appear in snapshot before its assertion date")
	}

	// No decay (frame historical, decay: none), so confidence should be stable.
	if b.CurrentConfidence < 0.74 || b.CurrentConfidence > 0.76 {
		t.Errorf("expected confidence ~0.75, got %.4f", b.CurrentConfidence)
	}
}

func TestCredalPriorLoadFile(t *testing.T) {
	src := `
frame epistemic
    composition: bayesian
    decay: none
    provenance-depth: 3
    imported-decay: most_conservative

record zombie-evidence in epistemic
    "Philosophical zombie argument"
    at: "2026-01-01T00:00:00Z"

believe hard-problem-real in epistemic
    "The hard problem is real"
    prior: [0.35, 0.65]
    from: zombie-evidence
`
	s := NewStore()
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := LoadFile(src, s, now); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	beliefs := s.AllBeliefs(now)
	if len(beliefs) != 1 {
		t.Fatalf("expected 1 belief, got %d", len(beliefs))
	}
	b := beliefs[0]
	// Midpoint of [0.35, 0.65] = 0.50
	if b.CurrentConfidence < 0.49 || b.CurrentConfidence > 0.51 {
		t.Errorf("expected midpoint confidence ~0.50, got %.4f", b.CurrentConfidence)
	}
	t.Logf("Credal prior belief loaded with confidence %.4f (midpoint of [0.35, 0.65])", b.CurrentConfidence)
}
