package lumen

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
)

// TextAnalysis holds the result of analyzing a natural language text
// for belief candidates.
type TextAnalysis struct {
	// Records are candidate empirical claims — statements that assert
	// something happened, was measured, or was found.
	Records []ExtractedRecord
	// Beliefs are candidate derived positions — statements that express
	// a position, interpretation, or conclusion.
	Beliefs []ExtractedBelief
	// Entities are named entities extracted from the text.
	Entities []string
	// Frame is the suggested frame for the extracted content.
	Frame string
}

// ExtractedRecord is a record candidate extracted from free text.
type ExtractedRecord struct {
	ID        string
	Content   string
	Confidence float64 // 1.0 for factual claims, lower for hedged ones
	Evidence  string  // the sentence or phrase it came from
}

// ExtractedBelief is a belief candidate extracted from free text.
type ExtractedBelief struct {
	ID         string
	Content    string
	Confidence float64
	Frame      string
	DerivedFrom []string // IDs of records it seems to derive from
	Evidence   string
}

// hedgePatterns are phrases that reduce confidence in a claim.
var hedgePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(might|may|could|possibly|perhaps|arguably|suggests?|seems? to|appears? to|tentatively|probably|likely|allegedly|reportedly)\b`),
	regexp.MustCompile(`(?i)\b(some evidence|limited evidence|preliminary|early|provisional|unclear)\b`),
	regexp.MustCompile(`(?i)\b(it is (possible|conceivable|plausible)|there is (some|limited) evidence)\b`),
}

// strongPatterns indicate high-confidence factual claims.
var strongPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(found that|demonstrated|showed|proved|confirmed|established|published|measured|observed|replicated)\b`),
	regexp.MustCompile(`(?i)\b(according to|the study|the experiment|the data|the results?|the evidence)\b`),
	regexp.MustCompile(`(?i)\b(in \d{4}|published in|journal|paper|study)\b`),
}

// positionPatterns indicate beliefs/positions rather than empirical records.
var positionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(therefore|thus|hence|consequently|it follows that|this (suggests?|implies?|means?|indicates?))\b`),
	regexp.MustCompile(`(?i)\b(I (believe|argue|think|hold|maintain)|we (believe|argue|maintain))\b`),
	regexp.MustCompile(`(?i)\b(the best explanation|most plausible|most likely|the evidence (supports?|suggests?))\b`),
	regexp.MustCompile(`(?i)\b(in conclusion|overall|taken together|on balance)\b`),
}

// negationPatterns detect negated claims.
var negationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(not|no|never|neither|nor|cannot|can't|doesn't|isn't|aren't|wasn't|weren't|fails? to|does not|did not)\b`),
}

// AnalyzeText extracts belief candidates from a natural language text.
// The text is split into sentences; each sentence is classified as a
// record candidate, belief candidate, or neither.
func AnalyzeText(text string) *TextAnalysis {
	sentences := splitSentences(text)
	analysis := &TextAnalysis{
		Frame: suggestFrame(text),
	}

	// Extract entities first — they inform belief-record linking
	entitySet := extractEntities(text)
	analysis.Entities = entitySet

	recordIdx := 0
	beliefIdx  := 0
	var recentRecordIDs []string

	for _, sent := range sentences {
		sent = strings.TrimSpace(sent)
		if len(sent) < 20 {
			continue
		}

		cls := ClassifyClaim(sent)
		if cls.Kind == ClaimUnknown {
			continue
		}

		// Apply hedge/strength modifiers on top of base confidence
		conf := cls.BaseConfidence
		for _, p := range hedgePatterns {
			if p.MatchString(sent) { conf -= 0.10 }
		}
		for _, p := range strongPatterns {
			if p.MatchString(sent) { conf += 0.05 }
		}
		if conf < 0.10 { conf = 0.10 }
		if conf > 0.95 { conf = 0.95 }

		// Use the classifier's frame suggestion if it's more specific than default
		frame := analysis.Frame
		if cls.SuggestedFrame != "empirical" {
			frame = cls.SuggestedFrame
		}

		if cls.IsRecord {
			id := fmt.Sprintf("rec-%03d", recordIdx)
			recordIdx++
			content := normalizeSentence(sent)
			// For attribution claims, prefix with attributee
			if cls.Kind == ClaimAttribution && cls.Attributee != "" {
				content = fmt.Sprintf("[%s] %s", cls.Attributee, content)
			}
			analysis.Records = append(analysis.Records, ExtractedRecord{
				ID:         id,
				Content:    content,
				Confidence: conf,
				Evidence:   sent,
			})
			recentRecordIDs = append(recentRecordIDs, id)
		} else {
			id := fmt.Sprintf("bel-%03d", beliefIdx)
			beliefIdx++
			sources := append([]string{}, recentRecordIDs...)
			if len(sources) > 3 {
				sources = sources[len(sources)-3:]
			}
			analysis.Beliefs = append(analysis.Beliefs, ExtractedBelief{
				ID:          id,
				Content:     normalizeSentence(sent),
				Confidence:  conf,
				Frame:       frame,
				DerivedFrom: sources,
				Evidence:    sent,
			})
		}
	}

	return analysis
}

// ToLumenFile converts a TextAnalysis into a .lm file string.
func (a *TextAnalysis) ToLumenFile(sourceName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Generated from: %s\n", sourceName)
	fmt.Fprintf(&b, "# Frame suggestion: %s\n", a.Frame)
	fmt.Fprintf(&b, "# Records: %d  Beliefs: %d  Entities: %d\n\n", len(a.Records), len(a.Beliefs), len(a.Entities))

	if len(a.Records) > 0 {
		fmt.Fprintf(&b, "# ── Records (empirical claims) ──\n\n")
		for _, r := range a.Records {
			fmt.Fprintf(&b, "record %s in %s\n", r.ID, a.Frame)
			fmt.Fprintf(&b, "  content: %q\n", r.Content)
			fmt.Fprintf(&b, "  # source: %q\n\n", truncate(r.Evidence, 80))
		}
	}

	if len(a.Beliefs) > 0 {
		fmt.Fprintf(&b, "# ── Beliefs (positions and conclusions) ──\n\n")
		for _, bel := range a.Beliefs {
			fmt.Fprintf(&b, "believe %s in %s\n", bel.ID, bel.Frame)
			fmt.Fprintf(&b, "  content: %q\n", bel.Content)
			fmt.Fprintf(&b, "  confidence: %.2f\n", bel.Confidence)
			if len(bel.DerivedFrom) > 0 {
				fmt.Fprintf(&b, "  from: %s\n", strings.Join(bel.DerivedFrom, " "))
			}
			fmt.Fprintf(&b, "  # source: %q\n\n", truncate(bel.Evidence, 80))
		}
	}

	return b.String()
}

// helpers

func splitSentences(text string) []string {
	// Split on sentence-ending punctuation, keeping the delimiter
	re := regexp.MustCompile(`[.!?]+\s+`)
	parts := re.Split(text, -1)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func isEmpiricalClaim(s string) bool {
	for _, p := range strongPatterns {
		if p.MatchString(s) {
			return true
		}
	}
	// Also catch year-anchored claims
	yearRe := regexp.MustCompile(`\b(19|20)\d{2}\b`)
	return yearRe.MatchString(s)
}

func isPositionClaim(s string) bool {
	for _, p := range positionPatterns {
		if p.MatchString(s) {
			return true
		}
	}
	return false
}

func baseConfidence(s string) float64 {
	conf := 0.70 // default

	// Hedges reduce confidence
	hedgeCount := 0
	for _, p := range hedgePatterns {
		if p.MatchString(s) {
			hedgeCount++
		}
	}
	conf -= float64(hedgeCount) * 0.12

	// Strong patterns increase confidence
	strongCount := 0
	for _, p := range strongPatterns {
		if p.MatchString(s) {
			strongCount++
		}
	}
	conf += float64(strongCount) * 0.08

	// Negation slightly reduces confidence (negated claims are less certain)
	for _, p := range negationPatterns {
		if p.MatchString(s) {
			conf -= 0.05
			break
		}
	}

	return math.Max(0.1, math.Min(0.95, conf))
}

func suggestFrame(text string) string {
	text = strings.ToLower(text)
	empiricalSignals := []string{"study", "experiment", "found", "measured", "published", "journal", "data", "results", "p=", "n=", "sample"}
	philosophySignals := []string{"consciousness", "qualia", "ontology", "epistemology", "metaphysics", "phenomenal", "intentionality"}
	reasoningSignals  := []string{"therefore", "thus", "it follows", "conclusion", "argument", "premise", "logic"}

	empirical, philosophy, reasoning := 0, 0, 0
	for _, sig := range empiricalSignals {
		if strings.Contains(text, sig) { empirical++ }
	}
	for _, sig := range philosophySignals {
		if strings.Contains(text, sig) { philosophy++ }
	}
	for _, sig := range reasoningSignals {
		if strings.Contains(text, sig) { reasoning++ }
	}

	switch {
	case philosophy > empirical && philosophy > reasoning:
		return "philosophical"
	case empirical > philosophy && empirical > reasoning:
		return "empirical"
	case reasoning > 0:
		return "reasoning"
	default:
		return "empirical"
	}
}

func extractEntities(text string) []string {
	// Reuse the SimpleNER approach from entity.go
	words := strings.Fields(text)
	seen  := make(map[string]bool)
	var entities []string

	for i, w := range words {
		// Capitalized words not at sentence start
		if i == 0 {
			continue
		}
		if len(w) < 3 {
			continue
		}
		clean := strings.Trim(w, ".,;:!?()'\"")
		if clean == "" {
			continue
		}
		if unicode.IsUpper(rune(clean[0])) && !seen[clean] {
			seen[clean] = true
			entities = append(entities, clean)
		}
	}
	return entities
}

func normalizeSentence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, ".!?")
	if len(s) > 0 {
		r := []rune(s)
		r[0] = unicode.ToUpper(r[0])
		s = string(r)
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
