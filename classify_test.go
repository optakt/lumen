package lumen

import "testing"

func TestClassifyAttribution(t *testing.T) {
	cases := []struct {
		sentence    string
		wantKind    ClaimKind
		wantAttrib  string
	}{
		{"Chalmers argues that consciousness has irreducible subjective character.", ClaimAttribution, "Chalmers"},
		{"Dennett claims that qualia are a philosophical confusion.", ClaimAttribution, "Dennett"},
		{"The study found significant correlation.", ClaimMeasurement, ""},
		{"The evidence suggests a new direction.", ClaimInference, ""}, // "The" excluded
		{"Therefore consciousness is fundamental.", ClaimInference, ""},
	}
	for _, tc := range cases {
		cls := ClassifyClaim(tc.sentence)
		if cls.Kind != tc.wantKind {
			t.Errorf("ClassifyClaim(%q) = %s, want %s", tc.sentence, cls.KindName, ClaimKindName(tc.wantKind))
		}
		if tc.wantAttrib != "" && cls.Attributee != tc.wantAttrib {
			t.Errorf("Attributee(%q) = %q, want %q", tc.sentence, cls.Attributee, tc.wantAttrib)
		}
		t.Logf("%-60q → %-15s conf=%.2f", tc.sentence[:min3(60, len(tc.sentence))], cls.KindName, cls.BaseConfidence)
	}
}

func TestClassifyMeasurement(t *testing.T) {
	cases := []string{
		"The experiment found n=256 subjects showed no correlation.",
		"The study measured phi and found no significant effect (p<0.05).",
		"Researchers observed a 23% reduction in IIT-predicted activity.",
	}
	for _, s := range cases {
		cls := ClassifyClaim(s)
		if cls.Kind != ClaimMeasurement {
			t.Errorf("expected Measurement for %q, got %s", s, cls.KindName)
		}
		if cls.BaseConfidence < 0.80 {
			t.Errorf("measurement confidence too low: %.2f for %q", cls.BaseConfidence, s)
		}
	}
}

func TestClassifyDefinition(t *testing.T) {
	s := "Consciousness is defined as the state of being aware of and able to think about one's existence."
	cls := ClassifyClaim(s)
	if cls.Kind != ClaimDefinition {
		t.Errorf("expected Definition, got %s", cls.KindName)
	}
	if cls.BaseConfidence < 0.90 {
		t.Errorf("definition confidence too low: %.2f", cls.BaseConfidence)
	}
}

func TestClassifyNormative(t *testing.T) {
	s := "AI systems ought to be transparent about their epistemic limitations."
	cls := ClassifyClaim(s)
	if cls.Kind != ClaimNormative {
		t.Errorf("expected Normative, got %s", cls.KindName)
	}
	if cls.SuggestedFrame != "reasoning" {
		t.Errorf("normative frame should be reasoning, got %s", cls.SuggestedFrame)
	}
}

func TestAnalyzeTextWithClassifier(t *testing.T) {
	text := `Chalmers argues that consciousness has irreducible subjective properties.
The Cogitate study measured activity in 256 subjects and found no IIT-predicted phi correlation.
Therefore, IIT is significantly weakened by these empirical results.
AI systems ought to be transparent about uncertainty in their outputs.`

	analysis := AnalyzeText(text)
	t.Logf("Records: %d, Beliefs: %d", len(analysis.Records), len(analysis.Beliefs))

	if len(analysis.Records) < 2 {
		t.Errorf("expected at least 2 records, got %d", len(analysis.Records))
	}
	if len(analysis.Beliefs) < 1 {
		t.Errorf("expected at least 1 belief, got %d", len(analysis.Beliefs))
	}

	// The normative claim should be a belief in reasoning frame
	foundNormative := false
	for _, b := range analysis.Beliefs {
		if b.Frame == "reasoning" {
			foundNormative = true
			t.Logf("Normative belief in reasoning frame: %q conf=%.2f", b.Content, b.Confidence)
		}
	}
	if !foundNormative {
		t.Error("expected a normative belief in reasoning frame")
	}
}

func ClaimKindName(k ClaimKind) string {
	switch k {
	case ClaimAttribution: return "attribution"
	case ClaimMeasurement: return "measurement"
	case ClaimCausal:      return "causal"
	case ClaimDefinition:  return "definition"
	case ClaimInference:   return "inference"
	case ClaimNormative:   return "normative"
	case ClaimNegation:    return "negation"
	case ClaimExistential: return "existential"
	default:               return "unknown"
	}
}

func min3(a, b int) int {
	if a < b { return a }
	return b
}
