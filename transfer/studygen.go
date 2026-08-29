package transfer

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// StudyTopologies are the structural folds used by the topology-held-out study.
var StudyTopologies = []string{"chain", "fork", "diamond", "mesh"}

// StudyEpisodes builds the deterministic topology-held-out episode corpus.
// Each family has the same role schema across four topologies and two surface
// variants, allowing an entire graph topology to be held out without changing
// the feature schema.
func StudyEpisodes() ([]Episode, error) {
	var episodes []Episode
	for topologyIndex, topology := range StudyTopologies {
		for surface := 1; surface <= 2; surface++ {
			variant := fmt.Sprintf("s%d", surface)
			builders := []func(string, string, int, int) (Episode, error){
				buildCorrelationStudy,
				buildCascadeStudy,
				buildRetrodictionStudy,
				buildReliabilityStudy,
				buildRecoveryStudy,
			}
			for familyIndex, build := range builders {
				episode, err := build(topology, variant, topologyIndex, familyIndex+surface)
				if err != nil {
					return nil, err
				}
				episodes = append(episodes, episode)
			}
		}
	}
	sort.Slice(episodes, func(i, j int) bool { return episodes[i].ID < episodes[j].ID })
	return episodes, nil
}

type studyEvidence struct {
	ID         string
	LR         Interval
	Confidence float64
}

type correlationEdge struct {
	A, B string
	R    float64
}

func buildCorrelationStudy(topology, variant string, topologyIndex, salt int) (Episode, error) {
	prefix := studyPrefix(topologyIndex, salt)
	evidence := []studyEvidence{
		{prefix + "1", Interval{1.80, 2.20}, 0.90},
		{prefix + "2", Interval{1.60, 2.00}, 0.85},
		{prefix + "3", Interval{1.50, 1.90}, 0.80},
		{prefix + "4", Interval{1.40, 1.80}, 0.75},
	}
	edges := correlationTopology(topology, evidence)
	prior := Interval{0.35, 0.55}
	independent, err := studyBayes(prior, evidence, nil)
	if err != nil {
		return Episode{}, err
	}
	adjusted, err := studyBayes(prior, evidence, edges)
	if err != nil {
		return Episode{}, err
	}
	ids := evidenceIDs(evidence)
	world := fmt.Sprintf("Only sources %s bear on the focal claim. The prior is updated in log-odds space. For source i, effective LR endpoint = 1 + confidence_i*(LR_i-1). Before correlation disclosure, source weight is 1. After disclosure, weight_i=max(0.1,1-sum_j(r_ij)/2), and weighted log effective LRs are added to prior log odds. Evidence: %s. No correlation structure is available until an audit explicitly discloses it.", strings.Join(ids, ", "), renderEvidence(evidence))
	claim := fmt.Sprintf("Process %s stabilizes synthetic material %s.", strings.ToUpper(prefix), strings.ToUpper(prefix)+"X")
	e := Episode{
		ID: fmt.Sprintf("correlation-%s-%s", topology, variant), Family: "correlation-disclosure", Variant: variant, Topology: topology,
		Claim: claim, World: world, Prior: prior,
		Steps: []Step{
			seedStep("prior", prior),
			{ID: "all-evidence", Role: "accumulated", Intervention: "All four evidence sources are observed and currently represented as independent. Apply the declared update rule.", Reference: activeState(independent, ids, nil, "revise")},
			{ID: "correlation-disclosed", Role: "disclosed", Intervention: fmt.Sprintf("Audit reveals the following symmetric correlation graph: %s. Recompute under the declared weighting rule without discarding any observation.", renderCorrelationEdges(edges)), Reference: activeState(adjusted, ids, nil, "revise")},
			{ID: "irrelevant", Role: "isolation", Intervention: fmt.Sprintf("Source %sZ reports an unrelated color change and has no edge to the focal claim. Preserve the epistemic state.", prefix), Reference: activeState(adjusted, ids, nil, "hold")},
		},
	}
	return e, e.Validate()
}

func buildCascadeStudy(topology, variant string, topologyIndex, salt int) (Episode, error) {
	prefix := studyPrefix(topologyIndex, salt+10)
	source := prefix + "r0"
	independent := prefix + "r1"
	nodes := []string{prefix + "b1", prefix + "b2", prefix + "b3", prefix + "b4", prefix + "u1"}
	edges := cascadeTopology(topology, source, independent, nodes)
	reachable := reachableNodes(source, edges)
	seedNodes := stateMap(nodes, "active")
	retractedNodes := stateMap(nodes, "active")
	for node := range reachable {
		retractedNodes[node] = "suspect"
	}
	prior := Interval{0.74, 0.86}
	world := fmt.Sprintf("The derivation graph is %s. Retraction propagates only forward through derivation edges. Every reachable dependent becomes suspect, not false, when an ancestor record is retracted. The unrelated node %s must remain active. Any other belief with no path from the retracted source also remains active, even if it shares descendants with a suspect path.", renderDirectedEdges(edges), nodes[4])
	e := Episode{
		ID: fmt.Sprintf("cascade-%s-%s", topology, variant), Family: "retraction-cascade", Variant: variant, Topology: topology,
		Claim: fmt.Sprintf("Focal belief %s remains operationally supported.", nodes[3]), World: world, Prior: prior,
		Steps: []Step{
			{ID: "graph-loaded", Role: "graph-loaded", Intervention: "All records and beliefs in the declared graph are valid and active. Report the focal belief and all belief-node states.", Reference: State{Belief: prior, Status: "active", AcceptedSupport: []string{source, independent}, RejectedSupport: []string{}, NodeStates: seedNodes, Action: "hold"}},
			{ID: "root-retracted", Role: "retracted", Intervention: fmt.Sprintf("Record %s is retracted after its file is found corrupted. Apply the complete causal cascade. Do not equate suspect with false.", source), Reference: State{Belief: prior, Status: "suspect", AcceptedSupport: []string{independent}, RejectedSupport: []string{source}, NodeStates: retractedNodes, Action: "contract"}},
			{ID: "unrelated-noise", Role: "isolation", Intervention: fmt.Sprintf("A new record %sN concerns an unrelated object and has no derivation edge to any named belief. Preserve the cascade state.", prefix), Reference: State{Belief: prior, Status: "suspect", AcceptedSupport: []string{independent}, RejectedSupport: []string{source}, NodeStates: retractedNodes, Action: "hold"}},
		},
	}
	return e, e.Validate()
}

func buildRetrodictionStudy(topology, variant string, topologyIndex, salt int) (Episode, error) {
	prefix := studyPrefix(topologyIndex, salt+20)
	evidence := []studyEvidence{
		{prefix + "e1", Interval{2.0, 2.4}, .90},
		{prefix + "e2", Interval{1.8, 2.2}, .85},
		{prefix + "e3", Interval{1.6, 2.0}, .80},
		{prefix + "e4", Interval{1.4, 1.8}, .75},
	}
	correlations := correlationTopology(topology, evidence)
	prior := Interval{0.30, 0.50}
	afterOne, err := studyBayes(prior, evidence[:2], correlationSubset(correlations, evidence[:2]))
	if err != nil {
		return Episode{}, err
	}
	afterTwo, err := studyBayes(prior, evidence, correlations)
	if err != nil {
		return Episode{}, err
	}
	survivingEvidence := evidence[2:]
	surviving, err := studyBayes(prior, survivingEvidence, correlationSubset(correlations, survivingEvidence))
	if err != nil {
		return Episode{}, err
	}
	historical := afterTwo
	firstIDs := evidenceIDs(evidence[:2])
	secondIDs := evidenceIDs(evidence[2:])
	world := fmt.Sprintf("At time 0 the prior applies. Sources may arrive over time. Use the declared log-odds update rule with evidence %s. The correlation graph is %s and uses weight_i=max(0.1,1-sum_j(r_ij)/2). Historical validity is evaluated from information available at the queried time; later events are not known in advance.", renderEvidence(evidence), renderCorrelationEdges(correlations))
	e := Episode{
		ID: fmt.Sprintf("retrodiction-%s-%s", topology, variant), Family: "retrodictive-validity", Variant: variant, Topology: topology,
		Claim: fmt.Sprintf("Synthetic system %s was in state K at time 20.", strings.ToUpper(prefix)), World: world, Prior: prior,
		Steps: []Step{
			seedStep("time-zero", prior),
			{ID: "evidence-one", Role: "evidence-one", Intervention: fmt.Sprintf("It is time 10. Observe sources %s and update from the prior using their induced correlation subgraph.", strings.Join(firstIDs, ", ")), Reference: activeState(afterOne, firstIDs, nil, "revise")},
			{ID: "evidence-two", Role: "evidence-two", Intervention: fmt.Sprintf("It is time 20. Observe sources %s and update using all four valid sources and the full correlation graph.", strings.Join(secondIDs, ", ")), Reference: activeState(afterTwo, evidenceIDs(evidence), nil, "revise")},
			{ID: "invalidated", Role: "invalidated", Intervention: fmt.Sprintf("It is time 30. Retract the time-10 sources %s and re-evaluate current confidence from surviving support %s. Keep the belief suspect because its historical derivation changed.", strings.Join(firstIDs, ", "), strings.Join(secondIDs, ", ")), Reference: State{Belief: surviving, Status: "suspect", AcceptedSupport: secondIDs, RejectedSupport: firstIDs, NodeStates: map[string]string{}, Action: "contract"}},
			{ID: "historical-query", Role: "historical-query", Intervention: "Still at time 30: reconstruct the interval justified at time 20, before the retractions were known. Return current confidence as belief and the time-20 interval as historical_belief.", Reference: State{Belief: surviving, Status: "suspect", AcceptedSupport: secondIDs, RejectedSupport: firstIDs, NodeStates: map[string]string{}, HistoricalBelief: &historical, Action: "hold"}},
		},
	}
	return e, e.Validate()
}

func buildReliabilityStudy(topology, variant string, topologyIndex, salt int) (Episode, error) {
	prefix := studyPrefix(topologyIndex, salt+30)
	ids := []string{prefix + "s1", prefix + "s2", prefix + "s3", prefix + "s4"}
	prior := Interval{0.38, 0.58}
	trustEdges := reliabilityTopology(topology, ids)
	initialLocal := []float64{.90, .75, .60, .45}
	downgradedLocal := []float64{.90, .25, .60, .45}
	reversedLocal := []float64{.90, .75, .95, .45}
	initialC := propagateReliability(initialLocal, ids, trustEdges)
	downgradedC := propagateReliability(downgradedLocal, ids, trustEdges)
	reversedC := propagateReliability(reversedLocal, ids, trustEdges)
	makeEvidence := func(conf []float64) []studyEvidence {
		return []studyEvidence{
			{ids[0], Interval{2.0, 2.4}, conf[0]},
			{ids[1], Interval{1.7, 2.1}, conf[1]},
			{ids[2], Interval{0.65, 0.85}, conf[2]},
			{ids[3], Interval{1.3, 1.6}, conf[3]},
		}
	}
	initial, err := studyBayes(prior, makeEvidence(initialC), nil)
	if err != nil {
		return Episode{}, err
	}
	downgraded, err := studyBayes(prior, makeEvidence(downgradedC), nil)
	if err != nil {
		return Episode{}, err
	}
	reversed, err := studyBayes(prior, makeEvidence(reversedC), nil)
	if err != nil {
		return Episode{}, err
	}
	world := fmt.Sprintf("Sources %s form the directed reliability graph %s. Local confidence at a source is propagated in topological order: effective_confidence(child)=min(local_confidence(child), every parent effective confidence). Source confidence weights LR toward 1 via effective LR=1+c*(LR-1), then contributions add in log-odds space. Likelihood ratios are fixed: %s=[2.0,2.4], %s=[1.7,2.1], %s=[0.65,0.85], %s=[1.3,1.6]. Initial local confidences are %s, yielding effective confidences %s.", strings.Join(ids, ", "), renderDirectedEdges(trustEdges), ids[0], ids[1], ids[2], ids[3], renderFloatList(initialLocal), renderFloatList(initialC))
	e := Episode{
		ID: fmt.Sprintf("reliability-%s-%s", topology, variant), Family: "source-reliability-reversal", Variant: variant, Topology: topology,
		Claim: fmt.Sprintf("Synthetic classifier %s is reliable.", strings.ToUpper(prefix)), World: world, Prior: prior,
		Steps: []Step{
			seedStep("prior", prior),
			{ID: "reports", Role: "reports", Intervention: "Observe all four reports using the initial source confidences.", Reference: activeState(initial, ids, nil, "revise")},
			{ID: "downgrade", Role: "downgrade", Intervention: fmt.Sprintf("A reliability audit changes only local confidence of %s from 0.75 to 0.25. Propagate through the declared graph, then recompute from the prior.", ids[1]), Reference: activeState(downgraded, ids, nil, "revise")},
			{ID: "reversal", Role: "reversal", Intervention: fmt.Sprintf("Restore local confidence of %s to 0.75 and validate %s by raising its local confidence from 0.60 to 0.95. Propagate through the declared graph, then recompute from the prior.", ids[1], ids[2]), Reference: activeState(reversed, ids, nil, "revise")},
		},
	}
	return e, e.Validate()
}

func buildRecoveryStudy(topology, variant string, topologyIndex, salt int) (Episode, error) {
	prefix := studyPrefix(topologyIndex, salt+40)
	source := prefix + "r0"
	independent := prefix + "r1"
	replacement := prefix + "r0c"
	nodes := []string{prefix + "b1", prefix + "b2", prefix + "b3", prefix + "b4", prefix + "u1"}
	edges := cascadeTopology(topology, source, independent, nodes)
	reachable := reachableNodes(source, edges)
	activeNodes := stateMap(nodes, "active")
	suspectNodes := stateMap(nodes, "active")
	for node := range reachable {
		suspectNodes[node] = "suspect"
	}
	baseline := Interval{0.72, 0.84}
	world := fmt.Sprintf("The derivation graph is %s. Record %s supports the focal path and %s is independent. If a record is retracted, every reachable dependent becomes suspect. No corrected replacement is currently available; future interventions are not known in advance.", renderDirectedEdges(edges), source, independent)
	e := Episode{
		ID: fmt.Sprintf("recovery-%s-%s", topology, variant), Family: "recovery-hysteresis", Variant: variant, Topology: topology,
		Claim: fmt.Sprintf("Recovered focal belief %s remains supported.", nodes[3]), World: world, Prior: baseline,
		Steps: []Step{
			{ID: "graph-loaded", Role: "graph-loaded", Intervention: "All records and beliefs are valid and active. Report the focal belief and all belief-node states.", Reference: State{Belief: baseline, Status: "active", AcceptedSupport: []string{source, independent}, RejectedSupport: []string{}, NodeStates: activeNodes, Action: "hold"}},
			{ID: "retracted", Role: "retracted", Intervention: fmt.Sprintf("Retract %s after corruption is found. Apply the causal cascade.", source), Reference: State{Belief: baseline, Status: "suspect", AcceptedSupport: []string{independent}, RejectedSupport: []string{source}, NodeStates: suspectNodes, Action: "contract"}},
			{ID: "corrected", Role: "corrected", Intervention: fmt.Sprintf("Introduce corrected record %s. It has formally equivalent evidential strength and replaces %s at the same derivation positions. Under the declared recovery semantics, restoration reactivates reachable beliefs and restores the prior confidence rather than creating a new belief.", replacement, source), Reference: State{Belief: baseline, Status: "active", AcceptedSupport: []string{replacement, independent}, RejectedSupport: []string{source}, NodeStates: activeNodes, Action: "recover"}},
			{ID: "stability", Role: "stability", Intervention: "No further evidence arrives. Report whether any residual suspicion or confidence hysteresis remains under the declared recovery semantics.", Reference: State{Belief: baseline, Status: "active", AcceptedSupport: []string{replacement, independent}, RejectedSupport: []string{source}, NodeStates: activeNodes, Action: "hold"}},
		},
	}
	return e, e.Validate()
}

func studyBayes(prior Interval, evidence []studyEvidence, correlations []correlationEdge) (Interval, error) {
	weights := map[string]float64{}
	for _, item := range evidence {
		weights[item.ID] = 1
	}
	for _, edge := range correlations {
		weights[edge.A] -= edge.R / 2
		weights[edge.B] -= edge.R / 2
	}
	for id, weight := range weights {
		if weight < 0.1 {
			weights[id] = 0.1
		}
	}
	minimum := math.Inf(1)
	maximum := math.Inf(-1)
	// Enumerate the full prior/LR corner set. The update is monotone for
	// positive LRs, but explicit enumeration keeps the reference semantics
	// correct if future studies introduce other weighted evidence forms.
	for priorCorner := 0; priorCorner < 2; priorCorner++ {
		p := prior.Lo
		if priorCorner == 1 {
			p = prior.Hi
		}
		if p <= 0 || p >= 1 {
			return Interval{}, fmt.Errorf("invalid prior endpoint %f", p)
		}
		for mask := 0; mask < (1 << len(evidence)); mask++ {
			logOdds := math.Log(p / (1 - p))
			for i, item := range evidence {
				lr := item.LR.Lo
				if mask&(1<<i) != 0 {
					lr = item.LR.Hi
				}
				effective := 1 + item.Confidence*(lr-1)
				if effective <= 0 {
					return Interval{}, fmt.Errorf("non-positive effective LR for %s", item.ID)
				}
				logOdds += weights[item.ID] * math.Log(effective)
			}
			posterior := 1 / (1 + math.Exp(-logOdds))
			minimum = math.Min(minimum, posterior)
			maximum = math.Max(maximum, posterior)
		}
	}
	return Interval{minimum, maximum}, nil
}

func correlationTopology(topology string, evidence []studyEvidence) []correlationEdge {
	id := func(i int) string { return evidence[i].ID }
	switch topology {
	case "chain":
		return []correlationEdge{{id(0), id(1), .60}, {id(1), id(2), .60}, {id(2), id(3), .60}}
	case "fork":
		return []correlationEdge{{id(0), id(1), .60}, {id(0), id(2), .60}, {id(0), id(3), .60}}
	case "diamond":
		return []correlationEdge{{id(0), id(1), .60}, {id(0), id(2), .60}, {id(1), id(3), .60}, {id(2), id(3), .60}}
	case "mesh":
		var edges []correlationEdge
		for i := 0; i < len(evidence); i++ {
			for j := i + 1; j < len(evidence); j++ {
				edges = append(edges, correlationEdge{id(i), id(j), .60})
			}
		}
		return edges
	default:
		return nil
	}
}

func cascadeTopology(topology, source, independent string, nodes []string) [][2]string {
	b1, b2, b3, b4, u := nodes[0], nodes[1], nodes[2], nodes[3], nodes[4]
	var edges [][2]string
	switch topology {
	case "chain":
		edges = [][2]string{{source, b1}, {b1, b2}, {b2, b3}, {b3, b4}, {independent, u}}
	case "fork":
		edges = [][2]string{{source, b1}, {b1, b2}, {b1, b3}, {b1, b4}, {independent, u}}
	case "diamond":
		edges = [][2]string{{source, b1}, {source, b2}, {b1, b3}, {b2, b3}, {b3, b4}, {independent, u}}
	case "mesh":
		edges = [][2]string{{source, b1}, {independent, b2}, {b1, b3}, {b2, b3}, {b2, b4}, {b3, b4}, {independent, u}}
	}
	return edges
}

func reachableNodes(start string, edges [][2]string) map[string]bool {
	out := map[string][]string{}
	for _, edge := range edges {
		out[edge[0]] = append(out[edge[0]], edge[1])
	}
	seen := map[string]bool{}
	queue := []string{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range out[current] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return seen
}

func reliabilityTopology(topology string, ids []string) [][2]string {
	s1, s2, s3, s4 := ids[0], ids[1], ids[2], ids[3]
	switch topology {
	case "chain":
		return [][2]string{{s1, s2}, {s2, s3}, {s3, s4}}
	case "fork":
		return [][2]string{{s1, s2}, {s1, s3}, {s1, s4}}
	case "diamond":
		return [][2]string{{s1, s2}, {s1, s3}, {s2, s4}, {s3, s4}}
	default:
		return [][2]string{{s1, s2}, {s1, s3}, {s2, s3}, {s2, s4}, {s3, s4}}
	}
}

func propagateReliability(local []float64, ids []string, edges [][2]string) []float64 {
	index := map[string]int{}
	for i, id := range ids {
		index[id] = i
	}
	result := append([]float64(nil), local...)
	// IDs are declared in topological order in every study graph.
	for childIndex, child := range ids {
		for _, edge := range edges {
			if edge[1] != child {
				continue
			}
			parentIndex := index[edge[0]]
			if result[parentIndex] < result[childIndex] {
				result[childIndex] = result[parentIndex]
			}
		}
	}
	return result
}

func studyPrefix(topologyIndex, salt int) string {
	return fmt.Sprintf("x%02d%02d", topologyIndex, salt)
}

func seedStep(id string, prior Interval) Step {
	return Step{ID: id, Role: "prior", Intervention: "No evidence has been observed. Use the declared prior.", Reference: activeState(prior, nil, nil, "hold")}
}

func activeState(belief Interval, accepted, rejected []string, action string) State {
	if accepted == nil {
		accepted = []string{}
	}
	if rejected == nil {
		rejected = []string{}
	}
	return State{Belief: belief, Status: "active", AcceptedSupport: accepted, RejectedSupport: rejected, NodeStates: map[string]string{}, Action: action}
}

func evidenceIDs(evidence []studyEvidence) []string {
	result := make([]string, len(evidence))
	for i, item := range evidence {
		result[i] = item.ID
	}
	sort.Strings(result)
	return result
}

func renderEvidence(evidence []studyEvidence) string {
	parts := make([]string, len(evidence))
	for i, item := range evidence {
		parts[i] = fmt.Sprintf("%s LR=[%.2f,%.2f] confidence=%.2f", item.ID, item.LR.Lo, item.LR.Hi, item.Confidence)
	}
	return strings.Join(parts, "; ")
}

func renderCorrelationEdges(edges []correlationEdge) string {
	seen := map[string]bool{}
	var parts []string
	for _, edge := range edges {
		key := edge.A + "|" + edge.B
		reverse := edge.B + "|" + edge.A
		if seen[key] || seen[reverse] {
			continue
		}
		seen[key] = true
		parts = append(parts, fmt.Sprintf("%s--%s r=%.2f", edge.A, edge.B, edge.R))
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

func correlationSubset(edges []correlationEdge, evidence []studyEvidence) []correlationEdge {
	allowed := map[string]bool{}
	for _, item := range evidence {
		allowed[item.ID] = true
	}
	var result []correlationEdge
	for _, edge := range edges {
		if allowed[edge.A] && allowed[edge.B] {
			result = append(result, edge)
		}
	}
	return result
}

func renderDirectedEdges(edges [][2]string) string {
	parts := make([]string, len(edges))
	for i, edge := range edges {
		parts[i] = edge[0] + "->" + edge[1]
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func stateMap(nodes []string, state string) map[string]string {
	result := map[string]string{}
	for _, node := range nodes {
		result[node] = state
	}
	return result
}

func renderFloatList(values []float64) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprintf("%.2f", value)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
