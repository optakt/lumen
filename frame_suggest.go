package lumen

import (
	"math"
	"sort"
	"strings"
)

// FrameSuggestion holds the result of frame classification.
type FrameSuggestion struct {
	// Frame is the recommended frame name.
	Frame string
	// Confidence is how confident the classifier is (0–1).
	Confidence float64
	// Scores is the score for each candidate frame.
	Scores map[string]float64
	// Reasoning is a brief explanation of the recommendation.
	Reasoning string
}

// frameSignal is a keyword/pattern and the frame it signals, with weight.
type frameSignal struct {
	pattern string
	frame   string
	weight  float64
}

// frameSignals are the patterns used to score each frame.
// Weights reflect how strongly the pattern implies the frame.
var frameSignals = []frameSignal{
	// Philosophical signals
	{`consciousness`, "philosophical", 0.8},
	{`qualia`, "philosophical", 1.0},
	{`phenomenal`, "philosophical", 0.9},
	{`ontology`, "philosophical", 0.9},
	{`epistemology`, "philosophical", 0.8},
	{`metaphysic`, "philosophical", 0.9},
	{`intentionality`, "philosophical", 0.8},
	{`a priori`, "philosophical", 0.9},
	{`analytic`, "philosophical", 0.6},
	{`modal`, "philosophical", 0.7},
	{`conceivability`, "philosophical", 0.9},
	{`hard problem`, "philosophical", 1.0},
	{`mind-body`, "philosophical", 0.9},
	{`substance dualism`, "philosophical", 1.0},
	{`eliminativism`, "philosophical", 0.9},
	{`functionalism`, "philosophical", 0.8},
	{`panpsychism`, "philosophical", 0.9},

	// Empirical signals
	{`study`, "empirical", 0.6},
	{`experiment`, "empirical", 0.7},
	{`measured`, "empirical", 0.8},
	{`observed`, "empirical", 0.7},
	{`published`, "empirical", 0.6},
	{`journal`, "empirical", 0.7},
	{`replicat`, "empirical", 0.8},
	{`sample size`, "empirical", 0.9},
	{`p-value`, "empirical", 1.0},
	{`p <`, "empirical", 0.9},
	{`n =`, "empirical", 0.9},
	{`correlation`, "empirical", 0.7},
	{`statistical`, "empirical", 0.8},
	{`effect size`, "empirical", 0.9},
	{`brain scan`, "empirical", 0.8},
	{`fmri`, "empirical", 0.9},
	{`eeg`, "empirical", 0.8},
	{`clinical`, "empirical", 0.7},
	{`trial`, "empirical", 0.6},
	{`cohort`, "empirical", 0.8},

	// Contemporary signals (field-state-dependent claims)
	{`current research`, "contemporary", 0.7},
	{`recent findings`, "contemporary", 0.7},
	{`as of`, "contemporary", 0.6},
	{`latest`, "contemporary", 0.6},
	{`state of the art`, "contemporary", 0.8},
	{`consensus`, "contemporary", 0.7},
	{`mainstream`, "contemporary", 0.7},
	{`prevailing view`, "contemporary", 0.8},
	{`current debate`, "contemporary", 0.8},
	{`ongoing`, "contemporary", 0.6},
	{`emerging`, "contemporary", 0.6},
	{`field has`, "contemporary", 0.7},

	// Reasoning signals
	{`therefore`, "reasoning", 0.7},
	{`thus`, "reasoning", 0.5},
	{`it follows`, "reasoning", 0.8},
	{`premise`, "reasoning", 0.8},
	{`conclusion`, "reasoning", 0.7},
	{`argument`, "reasoning", 0.6},
	{`entails`, "reasoning", 0.8},
	{`implies`, "reasoning", 0.7},
	{`if and only if`, "reasoning", 0.9},
	{`by definition`, "reasoning", 0.8},
	{`logically`, "reasoning", 0.8},
	{`deductive`, "reasoning", 0.9},
	{`inductive`, "reasoning", 0.8},
}

// SuggestFrame analyzes a text string and recommends the most appropriate
// Lumen frame for it.
//
// The recommendation is based on keyword scoring: each matched pattern
// contributes its weight to the candidate frame's score. The result includes
// all frame scores so the caller can inspect the confidence margin.
func SuggestFrame(text string) FrameSuggestion {
	lower := strings.ToLower(text)

	scores := map[string]float64{
		"philosophical": 0,
		"empirical":     0,
		"contemporary":  0,
		"reasoning":     0,
	}

	for _, sig := range frameSignals {
		if strings.Contains(lower, sig.pattern) {
			scores[sig.frame] += sig.weight
		}
	}

	// Normalize: convert raw scores to 0–1 via softmax
	softmax := softmaxMap(scores)

	// Find winner
	var best string
	var bestScore float64
	for frame, score := range softmax {
		if score > bestScore {
			bestScore = score
			best = frame
		}
	}

	// If all scores are zero, default to empirical
	total := 0.0
	for _, s := range scores { total += s }
	if total == 0 {
		return FrameSuggestion{
			Frame:      "empirical",
			Confidence: 0.4,
			Scores:     softmax,
			Reasoning:  "No strong signals detected; defaulting to empirical.",
		}
	}

	// Confidence has two components:
	// 1. Margin (relative): how much more the winner scored than the runner-up.
	//    Softmax inflates this — even one keyword hit gives a high relative score
	//    when all other frames score zero. So margin alone overstates certainty.
	// 2. Coverage (absolute): total raw signal weight relative to text length.
	//    Caps confidence when the text has little signal regardless of who wins.
	secondBest := 0.0
	for frame, score := range softmax {
		if frame != best && score > secondBest {
			secondBest = score
		}
	}
	margin := bestScore - secondBest

	// Raw signal weight for the winning frame
	winnerRaw := scores[best]
	// Coverage: scales from 0 at no signal to 1 at 3+ strong signals (sum ≥ 2.0)
	coverage := math.Min(winnerRaw/2.0, 1.0)

	// Combined confidence: margin-based score scaled by coverage
	marginConf := math.Min(0.5+margin, 0.95)
	confidence := marginConf * (0.4 + 0.6*coverage) // minimum 0.4 of margin-conf even with zero coverage

	reasoning := buildReasoning(best, scores, lower)

	return FrameSuggestion{
		Frame:      best,
		Confidence: confidence,
		Scores:     softmax,
		Reasoning:  reasoning,
	}
}

// SuggestFrameForBelief suggests a frame for an existing belief in the store,
// taking into account both the belief's content and the frame its sources use.
// Source frame agreement increases confidence; disagreement decreases it.
func (s *Store) SuggestFrameForBelief(beliefID string) FrameSuggestion {
	s.mu.RLock()
	b, ok := s.beliefs[beliefID]
	if !ok {
		s.mu.RUnlock()
		return SuggestFrame("") // empty
	}
	content := b.Content
	derivation := append([]string{}, b.Derivation...)
	currentFrame := b.Frame
	s.mu.RUnlock()

	suggestion := SuggestFrame(content)

	// Check source frames for agreement
	if len(derivation) > 0 {
		s.mu.RLock()
		sourceFrames := make(map[string]int)
		for _, srcID := range derivation {
			if rec, ok := s.records[srcID]; ok {
				sourceFrames[rec.Frame]++
			} else if src, ok := s.beliefs[srcID]; ok {
				sourceFrames[src.Frame]++
			}
		}
		s.mu.RUnlock()

		// If sources predominantly agree with the suggestion, boost confidence
		agreeing := sourceFrames[suggestion.Frame]
		if agreeing > 0 {
			boost := float64(agreeing) / float64(len(derivation)) * 0.1
			suggestion.Confidence = math.Min(suggestion.Confidence+boost, 0.95)
			suggestion.Reasoning += " Source frames agree with suggestion."
		}

		// If current frame matches most source frames but differs from suggestion,
		// add a note
		var dominantSource string
		var dominantCount int
		for f, c := range sourceFrames {
			if c > dominantCount {
				dominantCount = c
				dominantSource = f
			}
		}
		if dominantSource != "" && dominantSource != suggestion.Frame {
			suggestion.Reasoning += " Note: most sources use frame " + dominantSource + "; consider that frame too."
		}
	}

	// Note if suggestion differs from current frame
	if currentFrame != "" && currentFrame != suggestion.Frame {
		suggestion.Reasoning += " Currently in " + currentFrame + " frame."
	}

	return suggestion
}

func softmaxMap(scores map[string]float64) map[string]float64 {
	// Collect keys in stable order for reproducibility
	keys := make([]string, 0, len(scores))
	for k := range scores { keys = append(keys, k) }
	sort.Strings(keys)

	// Compute max for numerical stability
	max := -math.MaxFloat64
	for _, v := range scores {
		if v > max { max = v }
	}
	if max < 0 { max = 0 }

	sum := 0.0
	exp := make(map[string]float64, len(scores))
	for _, k := range keys {
		e := math.Exp(scores[k] - max)
		exp[k] = e
		sum += e
	}
	result := make(map[string]float64, len(scores))
	for k, e := range exp {
		result[k] = e / sum
	}
	return result
}

func buildReasoning(frame string, scores map[string]float64, text string) string {
	// Find the top-scoring signals for the winning frame
	var matched []string
	for _, sig := range frameSignals {
		if sig.frame == frame && strings.Contains(text, sig.pattern) {
			matched = append(matched, `"`+sig.pattern+`"`)
			if len(matched) >= 3 { break }
		}
	}
	if len(matched) == 0 {
		return "Recommended " + frame + " frame based on overall content profile."
	}
	return "Recommended " + frame + " frame based on: " + strings.Join(matched, ", ") + "."
}
