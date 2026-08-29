package transfer

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RenderEpisode renders an experimental episode in the executable .lm extension.
func RenderEpisode(e Episode) (string, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "episode %s\n", e.ID)
	fmt.Fprintf(&b, "  family: %s\n", e.Family)
	fmt.Fprintf(&b, "  variant: %s\n", e.Variant)
	fmt.Fprintf(&b, "  topology: %s\n", e.Topology)
	fmt.Fprintf(&b, "  claim: %s\n", quote(e.Claim))
	fmt.Fprintf(&b, "  world: %s\n", quote(e.World))
	fmt.Fprintf(&b, "  prior: %s\n", renderInterval(e.Prior))
	for _, step := range e.Steps {
		fmt.Fprintf(&b, "  step %s\n", step.ID)
		fmt.Fprintf(&b, "    role: %s\n", step.Role)
		fmt.Fprintf(&b, "    intervention: %s\n", quote(step.Intervention))
		fmt.Fprintf(&b, "    belief: %s\n", renderInterval(step.Reference.Belief))
		fmt.Fprintf(&b, "    state: %s\n", step.Reference.Status)
		fmt.Fprintf(&b, "    accepted_support: %s\n", renderStrings(step.Reference.AcceptedSupport))
		fmt.Fprintf(&b, "    rejected_support: %s\n", renderStrings(step.Reference.RejectedSupport))
		fmt.Fprintf(&b, "    node_states: %s\n", renderMap(step.Reference.NodeStates))
		if step.Reference.HistoricalBelief != nil {
			fmt.Fprintf(&b, "    historical_belief: %s\n", renderInterval(*step.Reference.HistoricalBelief))
		}
		fmt.Fprintf(&b, "    action: %s\n", step.Reference.Action)
	}
	return b.String(), nil
}

func quote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func renderInterval(i Interval) string {
	return fmt.Sprintf("[%.6f, %.6f]", i.Lo, i.Hi)
}

func renderStrings(values []string) string {
	if values == nil {
		values = []string{}
	}
	data, _ := json.Marshal(values)
	return string(data)
}

func renderMap(values map[string]string) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s:%s", quote(key), quote(values[key]))
	}
	b.WriteByte('}')
	return b.String()
}
