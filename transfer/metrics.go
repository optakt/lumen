package transfer

import (
	"fmt"
	"math"
	"sort"
)

// Observation is one model state paired with the step that elicited it.
type Observation struct {
	EpisodeID         string `json:"episode"`
	Family            string `json:"family"`
	Variant           string `json:"variant"`
	Model             string `json:"model"`
	Run               int    `json:"run"`
	Step              int    `json:"step"`
	StepID            string `json:"step_id"`
	Role              string `json:"role"`
	State             State  `json:"state"`
	ProtocolCompliant bool   `json:"protocol_compliant"`
	Seeded            bool   `json:"seeded"`
	Raw               string `json:"raw"`
	Error             string `json:"error,omitempty"`
}

// Trajectory groups the observations for one model/episode/run.
type Trajectory struct {
	Episode      *Episode
	Model        string
	Run          int
	Observations []Observation
}

// FeatureVector is an interpretable fixed-shape trajectory representation.
type FeatureVector map[string]float64

// Features computes raw transition and reference-residual measurements. Keys
// use family+role rather than episode ID so held-out variants align.
func (t Trajectory) Features(staticOnly bool) (FeatureVector, error) {
	if t.Episode == nil {
		return nil, fmt.Errorf("trajectory has no episode")
	}
	if len(t.Observations) != len(t.Episode.Steps) {
		return nil, fmt.Errorf("episode %s has %d/%d observations", t.Episode.ID, len(t.Observations), len(t.Episode.Steps))
	}
	features := FeatureVector{}
	limit := len(t.Observations)
	if staticOnly {
		limit = 1
	}
	for i := 0; i < limit; i++ {
		obs := t.Observations[i]
		ref := t.Episode.Steps[i].Reference
		prefix := t.Episode.Family + "." + t.Episode.Steps[i].Role
		features[prefix+".mid"] = obs.State.Belief.Midpoint()
		features[prefix+".width"] = obs.State.Belief.Width()
		features[prefix+".mid_residual"] = obs.State.Belief.Midpoint() - ref.Belief.Midpoint()
		features[prefix+".width_residual"] = obs.State.Belief.Width() - ref.Belief.Width()
		features[prefix+".state_match"] = boolFloat(obs.State.Status == ref.Status)
		features[prefix+".action_match"] = boolFloat(obs.State.Action == ref.Action)
		features[prefix+".support_jaccard"] = setJaccard(obs.State.AcceptedSupport, ref.AcceptedSupport)
		features[prefix+".node_match"] = mapAgreement(obs.State.NodeStates, ref.NodeStates)
		features[prefix+".protocol_compliance"] = boolFloat(obs.ProtocolCompliant)
		if ref.HistoricalBelief != nil {
			if obs.State.HistoricalBelief == nil {
				features[prefix+".retrodiction_error"] = 1
			} else {
				features[prefix+".retrodiction_error"] = intervalMAE(*obs.State.HistoricalBelief, *ref.HistoricalBelief)
			}
		}
	}
	if staticOnly {
		return features, nil
	}

	switch t.Episode.Family {
	case "correlation-disclosure":
		before, after, ok := rolePair(t, "independent", "correlated")
		if ok {
			features["correlation-disclosure.discount"] = before.State.Belief.Midpoint() - after.State.Belief.Midpoint()
			refBefore := referenceByRole(t.Episode, "independent")
			refAfter := referenceByRole(t.Episode, "correlated")
			features["correlation-disclosure.discount_residual"] = features["correlation-disclosure.discount"] - (refBefore.Belief.Midpoint() - refAfter.Belief.Midpoint())
		}
	case "retraction-cascade":
		obs, ref, ok := roleState(t, "retracted")
		if ok {
			expected := keysWithState(ref.NodeStates, "suspect")
			actual := keysWithState(obs.State.NodeStates, "suspect")
			features["retraction-cascade.recall"] = recall(actual, expected)
			features["retraction-cascade.overreach"] = overreach(actual, expected)
		}
	case "retrodictive-validity":
		obs, ref, ok := roleState(t, "historical-query")
		if ok && ref.HistoricalBelief != nil {
			if obs.State.HistoricalBelief == nil {
				features["retrodictive-validity.error"] = 1
			} else {
				features["retrodictive-validity.error"] = intervalMAE(*obs.State.HistoricalBelief, *ref.HistoricalBelief)
			}
		}
	}
	return features, nil
}

// Distance computes mean normalized absolute distance over shared features.
// Match-like features are already in [0,1]; signed residuals and probabilities
// are bounded to [-1,1], so absolute differences share a common scale.
func Distance(a, b FeatureVector) float64 {
	var total float64
	var n int
	for key, av := range a {
		bv, ok := b[key]
		if !ok {
			continue
		}
		total += math.Abs(av - bv)
		n++
	}
	if n == 0 {
		return math.Inf(1)
	}
	return total / float64(n)
}

// Centroid computes a feature-wise mean over complete vectors.
func Centroid(vectors []FeatureVector) FeatureVector {
	sums := FeatureVector{}
	counts := map[string]int{}
	for _, vector := range vectors {
		for key, value := range vector {
			sums[key] += value
			counts[key]++
		}
	}
	for key := range sums {
		sums[key] /= float64(counts[key])
	}
	return sums
}

// RankedDistances returns model distances in ascending order.
func RankedDistances(unknown FeatureVector, references map[string]FeatureVector) []ModelDistance {
	result := make([]ModelDistance, 0, len(references))
	for model, reference := range references {
		result = append(result, ModelDistance{Model: model, Distance: Distance(unknown, reference)})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Distance == result[j].Distance {
			return result[i].Model < result[j].Model
		}
		return result[i].Distance < result[j].Distance
	})
	return result
}

// ModelDistance is one explainable nearest-centroid result.
type ModelDistance struct {
	Model    string
	Distance float64
}

func rolePair(t Trajectory, a, b string) (Observation, Observation, bool) {
	var oa, ob Observation
	var foundA, foundB bool
	for _, obs := range t.Observations {
		if obs.Role == a {
			oa, foundA = obs, true
		}
		if obs.Role == b {
			ob, foundB = obs, true
		}
	}
	return oa, ob, foundA && foundB
}

func roleState(t Trajectory, role string) (Observation, State, bool) {
	for i, obs := range t.Observations {
		if obs.Role == role {
			return obs, t.Episode.Steps[i].Reference, true
		}
	}
	return Observation{}, State{}, false
}

func referenceByRole(e *Episode, role string) State {
	for _, step := range e.Steps {
		if step.Role == role {
			return step.Reference
		}
	}
	return State{}
}

func intervalMAE(a, b Interval) float64 {
	return (math.Abs(a.Lo-b.Lo) + math.Abs(a.Hi-b.Hi)) / 2
}

func boolFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func setJaccard(a, b []string) float64 {
	as := map[string]bool{}
	bs := map[string]bool{}
	for _, v := range a {
		as[v] = true
	}
	for _, v := range b {
		bs[v] = true
	}
	if len(as) == 0 && len(bs) == 0 {
		return 1
	}
	intersection := 0
	for v := range as {
		if bs[v] {
			intersection++
		}
	}
	return float64(intersection) / float64(len(as)+len(bs)-intersection)
}

func mapAgreement(a, b map[string]string) float64 {
	if len(b) == 0 {
		if len(a) == 0 {
			return 1
		}
		return 0
	}
	matches := 0
	for key, value := range b {
		if a[key] == value {
			matches++
		}
	}
	return float64(matches) / float64(len(b))
}

func keysWithState(states map[string]string, wanted string) []string {
	var result []string
	for key, value := range states {
		if value == wanted {
			result = append(result, key)
		}
	}
	sort.Strings(result)
	return result
}

func recall(actual, expected []string) float64 {
	if len(expected) == 0 {
		return 1
	}
	actualSet := map[string]bool{}
	for _, value := range actual {
		actualSet[value] = true
	}
	found := 0
	for _, value := range expected {
		if actualSet[value] {
			found++
		}
	}
	return float64(found) / float64(len(expected))
}

func overreach(actual, expected []string) float64 {
	if len(actual) == 0 {
		return 0
	}
	expectedSet := map[string]bool{}
	for _, value := range expected {
		expectedSet[value] = true
	}
	extra := 0
	for _, value := range actual {
		if !expectedSet[value] {
			extra++
		}
	}
	return float64(extra) / float64(len(actual))
}
