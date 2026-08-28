package lumen

import (
	"regexp"
	"strings"
)

// ClaimKind classifies the epistemic type of a sentence.
// Each kind carries different default confidence and frame suggestions.
type ClaimKind int

const (
	ClaimUnknown     ClaimKind = iota
	ClaimAttribution           // "X argues/claims/holds that P"
	ClaimMeasurement           // "The study found X = Y"
	ClaimCausal                // "X causes/leads to Y"
	ClaimDefinition            // "X is defined as / means Y"
	ClaimInference             // "Therefore / thus / it follows that"
	ClaimNormative             // "X should / ought to Y"
	ClaimNegation              // "X does not / fails to / is not Y"
	ClaimExistential           // "X exists / there are X"
)

// ClaimClassification holds the classification result for a sentence.
type ClaimClassification struct {
	Kind     ClaimKind
	KindName string
	// Attribution-specific: who is attributed
	Attributee string
	// BaseConfidence is the suggested confidence before hedge/strength adjustment.
	BaseConfidence float64
	// SuggestedFrame is the preferred frame for this claim type.
	SuggestedFrame string
	// IsRecord is true if this should be stored as a record (empirical).
	// False means it should be stored as a belief (derived/positional).
	IsRecord bool
}

// attributionSubjects matches typical attribution openings.
// Requires the subject to be a proper noun (not "The", "This", "That", "These", etc.)
var attributionSubjects = regexp.MustCompile(
	`^([A-Z][a-z]+(?:\s+[A-Z][a-z]+)?)\s+(argues?|claims?|holds?|maintains?|contends?|asserts?|proposes?|writes?|states?|notes?|observes?|concludes?|believes?|shows?)`,
)

// attributionExclusions are words that look like proper nouns but are common determiners/articles.
var attributionExclusions = map[string]bool{
	"The": true, "This": true, "That": true, "These": true, "Those": true,
	"Such": true, "Some": true, "All": true, "Both": true, "Each": true,
	"Many": true, "Most": true, "More": true, "Other": true, "Several": true,
	"Its": true, "Their": true, "Our": true,
}

var measurementPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(found that|measured|observed|recorded|detected|quantified|counted)\b`),
	regexp.MustCompile(`(?i)\b(n\s*=\s*\d+|p\s*[<>=]\s*0\.\d+|r\s*=\s*[-\d.]+|β\s*=|effect size)\b`),
	regexp.MustCompile(`(?i)\b(the (study|experiment|trial|survey|analysis) (found|showed|revealed|demonstrated))\b`),
	regexp.MustCompile(`(?i)\b(\d+%|\d+ percent|percentage of|proportion of)\b`),
}

var causalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(causes?|leads? to|results? in|produces?|generates?|triggers?|induces?|enables?|prevents?|inhibits?)\b`),
	regexp.MustCompile(`(?i)\b(is caused by|is produced by|is the result of|is due to|stems? from|arises? from)\b`),
	regexp.MustCompile(`(?i)\b(if .+, then|when .+, .+ (increases?|decreases?|changes?))\b`),
}

var definitionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(is defined as|means?|refers? to|is understood as|is the concept of|denotes?|signifies?)\b`),
	regexp.MustCompile(`(?i)\b(by (definition|convention|agreement))\b`),
}

var inferencePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(therefore|thus|hence|consequently|it follows that|in conclusion|on balance|taken together|overall|this (suggests?|implies?|means?|indicates?))`),
	regexp.MustCompile(`(?i)\b(the evidence (suggests?|supports?|indicates?)|we can (conclude|infer)|this (demonstrates?|shows?|reveals?))\b`),
}

var normativePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(should|ought to|must|need to|is required to|is obligated to|has a duty to)\b`),
	regexp.MustCompile(`(?i)\b(is (right|wrong|good|bad|just|unjust|ethical|unethical|permissible|impermissible))\b`),
	regexp.MustCompile(`(?i)\b(we (should|ought|must)|one (should|must|ought))\b`),
}

var existentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(there (is|are|exist[s]?)|exists?|has been (found|shown) to exist)\b`),
}

var negationClassPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(does not|is not|are not|cannot|never|fails? to|did not|was not|were not|has not|have not)\b`),
}

// ClassifyClaim classifies a sentence into an epistemic claim type.
func ClassifyClaim(sentence string) ClaimClassification {
	s := strings.TrimSpace(sentence)

	// Measurement: checked before attribution to handle "Researchers found/observed/measured"
	for _, p := range measurementPatterns {
		if p.MatchString(s) {
			return ClaimClassification{
				Kind:           ClaimMeasurement,
				KindName:       "measurement",
				BaseConfidence: 0.85,
				SuggestedFrame: "empirical",
				IsRecord:       true,
			}
		}
	}

	// Attribution: "Chalmers argues that..."
	if m := attributionSubjects.FindStringSubmatch(s); m != nil {
		// Exclude common determiners that start sentences
		subject := m[1]
		firstWord := strings.Fields(subject)[0]
		if !attributionExclusions[firstWord] {
			return ClaimClassification{
				Kind:           ClaimAttribution,
				KindName:       "attribution",
				Attributee:     subject,
				BaseConfidence: 0.60,
				SuggestedFrame: "empirical",
				IsRecord:       true,
			}
		}
	}

	// Inference: "therefore...", "thus..."
	for _, p := range inferencePatterns {
		if p.MatchString(s) {
			return ClaimClassification{
				Kind:           ClaimInference,
				KindName:       "inference",
				BaseConfidence: 0.65,
				SuggestedFrame: "reasoning",
				IsRecord:       false,
			}
		}
	}

	// Normative
	for _, p := range normativePatterns {
		if p.MatchString(s) {
			return ClaimClassification{
				Kind:           ClaimNormative,
				KindName:       "normative",
				BaseConfidence: 0.55,
				SuggestedFrame: "reasoning",
				IsRecord:       false,
			}
		}
	}

	// Definition
	for _, p := range definitionPatterns {
		if p.MatchString(s) {
			return ClaimClassification{
				Kind:           ClaimDefinition,
				KindName:       "definition",
				BaseConfidence: 0.95, // definitional claims are (near-)tautological
				SuggestedFrame: "philosophical",
				IsRecord:       true,
			}
		}
	}

	// Causal
	for _, p := range causalPatterns {
		if p.MatchString(s) {
			return ClaimClassification{
				Kind:           ClaimCausal,
				KindName:       "causal",
				BaseConfidence: 0.70,
				SuggestedFrame: "empirical",
				IsRecord:       true,
			}
		}
	}

	// Existential
	for _, p := range existentialPatterns {
		if p.MatchString(s) {
			return ClaimClassification{
				Kind:           ClaimExistential,
				KindName:       "existential",
				BaseConfidence: 0.75,
				SuggestedFrame: "empirical",
				IsRecord:       true,
			}
		}
	}

	// Negation (after causal to avoid misclassification)
	for _, p := range negationClassPatterns {
		if p.MatchString(s) {
			return ClaimClassification{
				Kind:           ClaimNegation,
				KindName:       "negation",
				BaseConfidence: 0.65,
				SuggestedFrame: "empirical",
				IsRecord:       true,
			}
		}
	}

	return ClaimClassification{
		Kind:           ClaimUnknown,
		KindName:       "unknown",
		BaseConfidence: 0.60,
		SuggestedFrame: "empirical",
		IsRecord:       false,
	}
}
