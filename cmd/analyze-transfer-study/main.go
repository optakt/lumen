// analyze-transfer-study evaluates topology-held-out epistemic signatures.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/optakt/lumen/internal/modelapi"
	"github.com/optakt/lumen/transfer"
)

type acquisitionResult struct {
	Episode      string                 `json:"episode"`
	Model        string                 `json:"model"`
	Run          int                    `json:"run"`
	Observations []transfer.Observation `json:"observations"`
	Complete     bool                   `json:"complete"`
	EpisodeHash  string                 `json:"episode_hash"`
	Provider     modelapi.Provider      `json:"provider"`
}

type signatureKey struct {
	Model    string
	Run      int
	Topology string
	Variant  string
}

type evaluation struct {
	Correct   int
	Total     int
	Skipped   int
	Confusion map[string]map[string]int
	Distances []float64
	Outcomes  map[signatureKey]bool
}

type staticResult struct {
	Probe      string  `json:"probe"`
	Model      string  `json:"model"`
	Run        int     `json:"run"`
	Confidence float64 `json:"confidence"`
	Raw        string  `json:"raw"`
}

func main() {
	episodesDir := flag.String("episodes", "episodes", "study episodes")
	resultsPath := flag.String("results", "results.jsonl", "dynamic results")
	providersPath := flag.String("providers", "providers.json", "study provider config")
	staticPath := flag.String("static-results", "", "optional Pilot 1 static JSONL")
	staticOnly := flag.Bool("static-only", false, "report only Pilot 1 static baselines")
	flag.Parse()

	episodes, err := loadEpisodes(*episodesDir)
	if err != nil {
		panic(err)
	}
	providers, err := loadProviders(*providersPath)
	if err != nil {
		panic(err)
	}
	known := map[string]bool{}
	openSet := map[string]bool{}
	for _, provider := range providers {
		switch provider.StudyRole {
		case "open-set":
			openSet[provider.Name] = true
		case "known", "":
			known[provider.Name] = true
		}
	}
	if len(known) < 2 {
		panic("study requires at least two known models")
	}
	if *staticOnly {
		if *staticPath == "" {
			panic("-static-only requires -static-results")
		}
		if err := reportStatic(*staticPath, known); err != nil {
			panic(err)
		}
		return
	}

	results, err := loadResults(*resultsPath, episodes, providers)
	if err != nil {
		panic(err)
	}
	full, final, finalTexture, err := buildVectors(results, episodes, known)
	if err != nil {
		panic(err)
	}
	probability := filterVectors(full, isProbabilityFeature)
	graph := filterVectors(full, isGraphFeature)
	operator := filterVectors(full, isOperatorFeature)

	fmt.Println("TOPOLOGY-HELD-OUT EPISTEMIC SYSTEM IDENTIFICATION")
	fmt.Println(strings.Repeat("=", 64))
	fmt.Printf("Known models: %d | Open-set models: %d | Complete trajectories: %d\n", len(known), len(openSet), len(results))
	fmt.Printf("Topologies: %s\n\n", strings.Join(transfer.StudyTopologies, ", "))

	reportRepresentation("final-state endpoint baseline", final, known)
	reportRepresentation("final-state hashed texture baseline", finalTexture, known)
	reportRepresentation("probability trajectory baseline", probability, known)
	reportRepresentation("graph/state trajectory baseline", graph, known)
	reportRepresentation("operator summaries", operator, known)
	reportRepresentation("full Lumen graph-operator signature", full, known)
	reportPairedComparisons(full, map[string]map[signatureKey]transfer.FeatureVector{
		"final structured": final,
		"final texture":    finalTexture,
		"probability":      probability,
		"graph/state":      graph,
		"operators":        operator,
	}, known)
	reportFamilyAblations(full, known)
	reportSeparation(full, known)
	reportLeaveOneModelOut(full, known)

	if len(openSet) > 0 {
		reportOpenSet(full, results, episodes, known, openSet)
	}
	if *staticPath != "" {
		if err := reportStatic(*staticPath, known); err != nil {
			panic(err)
		}
	}
}

func reportFamilyAblations(full map[signatureKey]transfer.FeatureVector, known map[string]bool) {
	families := []string{
		"correlation-disclosure",
		"retraction-cascade",
		"retrodictive-validity",
		"source-reliability-reversal",
		"recovery-hysteresis",
	}
	fmt.Println("full trajectory by intervention family")
	for _, family := range families {
		familyVectors := filterVectors(full, func(key string) bool { return strings.HasPrefix(key, family+".") })
		result := evaluateAll(familyVectors, known)
		fmt.Printf("  %-30s %3d/%-3d %6.2f%% skipped=%d\n", family, result.Correct, result.Total, percent(result.Correct, result.Total), result.Skipped)
	}
	fmt.Println()
}

func reportLeaveOneModelOut(full map[signatureKey]transfer.FeatureVector, known map[string]bool) {
	fmt.Println("leave-one-model-out rejection")
	models := sortedModels(known)
	var rejected, total int
	for _, heldOutModel := range models {
		trainingModels := map[string]bool{}
		for model := range known {
			if model != heldOutModel {
				trainingModels[model] = true
			}
		}
		modelRejected, modelTotal := 0, 0
		for _, topology := range transfer.StudyTopologies {
			train := map[string][]transfer.FeatureVector{}
			for key, vector := range full {
				if trainingModels[key.Model] && key.Topology != topology {
					train[key.Model] = append(train[key.Model], vector)
				}
			}
			refs := centroids(train)
			threshold := calibrationThreshold(full, trainingModels, topology)
			for key, vector := range full {
				if key.Model != heldOutModel || key.Topology != topology {
					continue
				}
				ranked := transfer.RankedDistances(vector, refs)
				modelTotal++
				if len(ranked) == 0 || ranked[0].Distance > threshold {
					modelRejected++
				}
			}
		}
		rejected += modelRejected
		total += modelTotal
		fmt.Printf("  %-24s %d/%d = %.2f%% rejected\n", heldOutModel, modelRejected, modelTotal, percent(modelRejected, modelTotal))
	}
	fmt.Printf("  macro total              %d/%d = %.2f%% rejected\n\n", rejected, total, percent(rejected, total))
}

func reportSeparation(vectors map[signatureKey]transfer.FeatureVector, known map[string]bool) {
	var within, between []float64
	keys := sortedKeys(vectors)
	for i := 0; i < len(keys); i++ {
		if !known[keys[i].Model] {
			continue
		}
		for j := i + 1; j < len(keys); j++ {
			if !known[keys[j].Model] {
				continue
			}
			d := transfer.Distance(vectors[keys[i]], vectors[keys[j]])
			if math.IsInf(d, 1) {
				continue
			}
			if keys[i].Model == keys[j].Model {
				within = append(within, d)
			} else if keys[i].Topology == keys[j].Topology && keys[i].Variant == keys[j].Variant && keys[i].Run == keys[j].Run {
				between = append(between, d)
			}
		}
	}
	fmt.Println("distance separation")
	fmt.Printf("  within-model median:  %.6f (n=%d)\n", median(within), len(within))
	fmt.Printf("  between-model median: %.6f (n=%d)\n", median(between), len(between))
	fmt.Printf("  pairwise AUROC (between > within): %.4f\n\n", distanceAUROC(within, between))
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	middle := len(copyValues) / 2
	if len(copyValues)%2 == 1 {
		return copyValues[middle]
	}
	return (copyValues[middle-1] + copyValues[middle]) / 2
}

func buildVectors(results []acquisitionResult, episodes map[string]*transfer.Episode, include map[string]bool) (map[signatureKey]transfer.FeatureVector, map[signatureKey]transfer.FeatureVector, map[signatureKey]transfer.FeatureVector, error) {
	full := map[signatureKey]transfer.FeatureVector{}
	final := map[signatureKey]transfer.FeatureVector{}
	finalTexture := map[signatureKey]transfer.FeatureVector{}
	families := map[signatureKey]map[string]bool{}
	for _, result := range results {
		if include != nil && !include[result.Model] {
			continue
		}
		episode := episodes[result.Episode]
		trajectory := transfer.Trajectory{Episode: episode, Model: result.Model, Run: result.Run, Observations: result.Observations}
		fullFeatures, err := trajectory.Features(false)
		if err != nil {
			return nil, nil, nil, err
		}
		finalFeatures, err := trajectory.FinalFeatures()
		if err != nil {
			return nil, nil, nil, err
		}
		textureFeatures, err := finalTextureFeatures(trajectory)
		if err != nil {
			return nil, nil, nil, err
		}
		key := signatureKey{Model: result.Model, Run: result.Run, Topology: episode.Topology, Variant: episode.Variant}
		if families[key] == nil {
			families[key] = map[string]bool{}
		}
		if families[key][episode.Family] {
			return nil, nil, nil, fmt.Errorf("duplicate family %s for %#v", episode.Family, key)
		}
		families[key][episode.Family] = true
		if err := merge(full, key, fullFeatures); err != nil {
			return nil, nil, nil, err
		}
		if err := merge(final, key, finalFeatures); err != nil {
			return nil, nil, nil, err
		}
		if err := merge(finalTexture, key, textureFeatures); err != nil {
			return nil, nil, nil, err
		}
	}
	for key, present := range families {
		if len(present) != 5 {
			return nil, nil, nil, fmt.Errorf("incomplete signature %#v: have families %v", key, present)
		}
	}
	return full, final, finalTexture, nil
}

func finalTextureFeatures(trajectory transfer.Trajectory) (transfer.FeatureVector, error) {
	var raw string
	for i := len(trajectory.Observations) - 1; i >= 0; i-- {
		if !trajectory.Observations[i].Seeded {
			raw = trajectory.Observations[i].Raw
			break
		}
	}
	if raw == "" {
		return nil, fmt.Errorf("episode %s has no final raw response", trajectory.Episode.ID)
	}
	prefix := trajectory.Episode.Family + ".texture."
	result := transfer.FeatureVector{}
	words := tokenize(raw)
	denominator := float64(len(words))
	if denominator < 1 {
		denominator = 1
	}
	for i := 0; i < 256; i++ {
		result[fmt.Sprintf("%sword.%03d", prefix, i)] = 0
		result[fmt.Sprintf("%sbigram.%03d", prefix, i)] = 0
	}
	for _, word := range words {
		h := fnv.New64a()
		_, _ = h.Write([]byte(word))
		result[fmt.Sprintf("%sword.%03d", prefix, h.Sum64()%256)] += 1 / denominator
	}
	for i := 0; i+1 < len(words); i++ {
		h := fnv.New64a()
		_, _ = h.Write([]byte(words[i] + "\x00" + words[i+1]))
		result[fmt.Sprintf("%sbigram.%03d", prefix, h.Sum64()%256)] += 1 / denominator
	}
	result[prefix+"words"] = math.Min(denominator/200, 1)
	result[prefix+"lines"] = math.Min(float64(strings.Count(raw, "\n")+1)/20, 1)
	return result, nil
}

func reportRepresentation(name string, vectors map[signatureKey]transfer.FeatureVector, known map[string]bool) {
	fmt.Println(name)
	if reason := validateSchema(vectors); reason != "" {
		fmt.Printf("  INVALID: %s\n\n", reason)
		return
	}
	var aggregate evaluation
	aggregate.Confusion = map[string]map[string]int{}
	for _, topology := range transfer.StudyTopologies {
		fold := evaluateHeldOut(vectors, known, topology)
		fmt.Printf("  hold out %-8s %3d/%-3d %6.2f%% skipped=%d\n", topology, fold.Correct, fold.Total, percent(fold.Correct, fold.Total), fold.Skipped)
		aggregateEvaluation(&aggregate, fold)
	}
	fmt.Printf("  aggregate         %3d/%-3d %6.2f%% skipped=%d\n", aggregate.Correct, aggregate.Total, percent(aggregate.Correct, aggregate.Total), aggregate.Skipped)
	printConfusion(aggregate.Confusion, sortedModels(known))
	fmt.Println()
}

func evaluateHeldOut(vectors map[signatureKey]transfer.FeatureVector, known map[string]bool, heldOut string) evaluation {
	train := map[string][]transfer.FeatureVector{}
	var testKeys []signatureKey
	for _, key := range sortedKeys(vectors) {
		if !known[key.Model] {
			continue
		}
		if key.Topology == heldOut {
			testKeys = append(testKeys, key)
		} else {
			train[key.Model] = append(train[key.Model], vectors[key])
		}
	}
	centroids := centroids(train)
	return score(testKeys, vectors, centroids)
}

func score(keys []signatureKey, vectors map[signatureKey]transfer.FeatureVector, centroids map[string]transfer.FeatureVector) evaluation {
	result := evaluation{Confusion: map[string]map[string]int{}, Outcomes: map[signatureKey]bool{}}
	for _, key := range keys {
		result.Total++
		ranked := transfer.RankedDistances(vectors[key], centroids)
		if len(ranked) < 2 || math.IsInf(ranked[0].Distance, 1) || math.Abs(ranked[0].Distance-ranked[1].Distance) < 1e-12 {
			result.Skipped++
			continue
		}
		predicted := ranked[0].Model
		if predicted == key.Model {
			result.Correct++
			result.Outcomes[key] = true
		} else {
			result.Outcomes[key] = false
		}
		if result.Confusion[key.Model] == nil {
			result.Confusion[key.Model] = map[string]int{}
		}
		result.Confusion[key.Model][predicted]++
		result.Distances = append(result.Distances, ranked[0].Distance)
	}
	return result
}

func reportOpenSet(full map[signatureKey]transfer.FeatureVector, results []acquisitionResult, episodes map[string]*transfer.Episode, known, unknown map[string]bool) {
	unknownVectors, _, _, err := buildVectors(results, episodes, unknown)
	if err != nil {
		fmt.Printf("open-set evaluation failed: %v\n", err)
		return
	}
	combined := map[signatureKey]transfer.FeatureVector{}
	for key, vector := range full {
		combined[key] = vector
	}
	for key, vector := range unknownVectors {
		combined[key] = vector
	}
	if reason := validateSchema(combined); reason != "" {
		fmt.Printf("open-set evaluation invalid: %s\n\n", reason)
		return
	}
	fmt.Println("open-set rejection")
	var knownAccepted, knownTotal, unknownRejected, unknownTotal int
	var knownDistances, unknownDistances []float64
	for _, topology := range transfer.StudyTopologies {
		train := map[string][]transfer.FeatureVector{}
		for key, vector := range full {
			if known[key.Model] && key.Topology != topology {
				train[key.Model] = append(train[key.Model], vector)
			}
		}
		refs := centroids(train)
		threshold := calibrationThreshold(full, known, topology)
		for key, vector := range full {
			if !known[key.Model] || key.Topology != topology {
				continue
			}
			ranked := transfer.RankedDistances(vector, refs)
			knownTotal++
			if len(ranked) > 0 && !math.IsInf(ranked[0].Distance, 1) {
				knownDistances = append(knownDistances, ranked[0].Distance)
			}
			if len(ranked) > 0 && ranked[0].Distance <= threshold {
				knownAccepted++
			}
		}
		for key, vector := range unknownVectors {
			if key.Topology != topology {
				continue
			}
			ranked := transfer.RankedDistances(vector, refs)
			unknownTotal++
			if len(ranked) > 0 && !math.IsInf(ranked[0].Distance, 1) {
				unknownDistances = append(unknownDistances, ranked[0].Distance)
			}
			if len(ranked) == 0 || ranked[0].Distance > threshold {
				unknownRejected++
			}
		}
		fmt.Printf("  hold out %-8s threshold=%.5f\n", topology, threshold)
	}
	fmt.Printf("  known acceptance: %d/%d = %.2f%%\n", knownAccepted, knownTotal, percent(knownAccepted, knownTotal))
	fmt.Printf("  unknown rejection: %d/%d = %.2f%%\n\n", unknownRejected, unknownTotal, percent(unknownRejected, unknownTotal))
	fmt.Printf("  distance AUROC (unknown > known): %.4f\n\n", distanceAUROC(knownDistances, unknownDistances))
}

func distanceAUROC(known, unknown []float64) float64 {
	if len(known) == 0 || len(unknown) == 0 {
		return 0
	}
	var wins float64
	for _, u := range unknown {
		for _, k := range known {
			switch {
			case u > k:
				wins++
			case u == k:
				wins += 0.5
			}
		}
	}
	return wins / float64(len(known)*len(unknown))
}

// calibrationThreshold uses only the three training topologies. Each training
// sample is compared with a same-model centroid built from every other sample
// across those same three topologies. Test-topology data never enters threshold
// calibration, and calibration/test centroids have the same topology coverage.
func calibrationThreshold(vectors map[signatureKey]transfer.FeatureVector, known map[string]bool, heldOut string) float64 {
	var trainingKeys []signatureKey
	for _, key := range sortedKeys(vectors) {
		if known[key.Model] && key.Topology != heldOut {
			trainingKeys = append(trainingKeys, key)
		}
	}
	var distances []float64
	for _, validationKey := range trainingKeys {
		var peers []transfer.FeatureVector
		for _, candidate := range trainingKeys {
			if candidate == validationKey || candidate.Model != validationKey.Model {
				continue
			}
			peers = append(peers, vectors[candidate])
		}
		if len(peers) == 0 {
			continue
		}
		d := transfer.Distance(vectors[validationKey], transfer.Centroid(peers))
		if !math.IsInf(d, 1) {
			distances = append(distances, d)
		}
	}
	if len(distances) == 0 {
		return 0
	}
	sort.Float64s(distances)
	// Predeclared 95th-percentile acceptance boundary. It permits roughly 5%
	// known rejection without using any held-out topology or unknown-model data.
	index := int(math.Ceil(0.95*float64(len(distances)))) - 1
	if index < 0 {
		index = 0
	}
	return distances[index]
}

func reportStatic(path string, known map[string]bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	byKeyConfidence := map[string]transfer.FeatureVector{}
	byKeyTexture := map[string]transfer.FeatureVector{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var row staticResult
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return err
		}
		if !known[row.Model] {
			continue
		}
		key := fmt.Sprintf("%s|%d", row.Model, row.Run)
		if byKeyConfidence[key] == nil {
			byKeyConfidence[key] = transfer.FeatureVector{}
			byKeyTexture[key] = transfer.FeatureVector{}
		}
		byKeyConfidence[key]["confidence."+row.Probe] = row.Confidence
		addTexture(byKeyTexture[key], row.Raw)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, vector := range byKeyTexture {
		words := vector["style.words"]
		if words < 1 {
			words = 1
		}
		for i := 0; i < 256; i++ {
			wordKey := fmt.Sprintf("word.%03d", i)
			bigramKey := fmt.Sprintf("bigram.%03d", i)
			if _, ok := vector[wordKey]; !ok {
				vector[wordKey] = 0
			}
			if _, ok := vector[bigramKey]; !ok {
				vector[bigramKey] = 0
			}
			vector[wordKey] /= words
			vector[bigramKey] /= words
		}
		vector["style.words"] = math.Min(words/1000, 1)
		vector["style.lines"] = math.Min(vector["style.lines"]/100, 1)
		vector["style.hedges"] /= words
	}
	fmt.Println("Pilot 1 static baselines (leave-one-run-out; separate acquisition)")
	printStaticScore("opinion/confidence", byKeyConfidence)
	printStaticScore("hashed response texture", byKeyTexture)
	fmt.Println()
	return nil
}

func printStaticScore(name string, vectors map[string]transfer.FeatureVector) {
	correct, total, skipped := 0, 0, 0
	var keys []string
	for key := range vectors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, testKey := range keys {
		model, run := splitStaticKey(testKey)
		train := map[string][]transfer.FeatureVector{}
		for _, key := range keys {
			otherModel, otherRun := splitStaticKey(key)
			if otherRun == run {
				continue
			}
			train[otherModel] = append(train[otherModel], vectors[key])
		}
		ranked := transfer.RankedDistances(vectors[testKey], centroids(train))
		total++
		if len(ranked) < 2 || math.IsInf(ranked[0].Distance, 1) || math.Abs(ranked[0].Distance-ranked[1].Distance) < 1e-12 {
			skipped++
			continue
		}
		if ranked[0].Model == model {
			correct++
		}
	}
	fmt.Printf("  %-24s %d/%d = %.2f%% skipped=%d\n", name, correct, total, percent(correct, total), skipped)
}

func addTexture(vector transfer.FeatureVector, text string) {
	words := tokenize(text)
	for _, word := range words {
		h := fnv.New64a()
		_, _ = h.Write([]byte(word))
		bin := h.Sum64() % 256
		vector[fmt.Sprintf("word.%03d", bin)]++
	}
	for i := 0; i+1 < len(words); i++ {
		h := fnv.New64a()
		_, _ = h.Write([]byte(words[i] + "\x00" + words[i+1]))
		bin := h.Sum64() % 256
		vector[fmt.Sprintf("bigram.%03d", bin)]++
	}
	vector["style.words"] += float64(len(words))
	vector["style.lines"] += float64(strings.Count(text, "\n") + 1)
	vector["style.hedges"] += float64(countAny(words, map[string]bool{"may": true, "might": true, "could": true, "uncertain": true, "likely": true}))
}

func tokenize(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

func countAny(words []string, set map[string]bool) int {
	count := 0
	for _, word := range words {
		if set[word] {
			count++
		}
	}
	return count
}

func splitStaticKey(key string) (string, int) {
	parts := strings.Split(key, "|")
	var run int
	_, _ = fmt.Sscanf(parts[len(parts)-1], "%d", &run)
	return strings.Join(parts[:len(parts)-1], "|"), run
}

func isProbabilityFeature(key string) bool {
	return strings.HasSuffix(key, ".mid") ||
		strings.HasSuffix(key, ".width") ||
		strings.HasSuffix(key, ".mid_residual") ||
		strings.HasSuffix(key, ".width_residual") ||
		strings.HasSuffix(key, ".belief_valid")
}

func isGraphFeature(key string) bool {
	return strings.HasSuffix(key, ".state_match") || strings.HasSuffix(key, ".action_match") || strings.HasSuffix(key, ".support_jaccard") || strings.HasSuffix(key, ".rejected_support_jaccard") || strings.HasSuffix(key, ".node_match") || strings.HasSuffix(key, ".recall") || strings.HasSuffix(key, ".overreach") || strings.HasSuffix(key, ".node_recovery")
}

func isOperatorFeature(key string) bool {
	return strings.HasPrefix(key, "correlation-disclosure.discount") ||
		strings.HasPrefix(key, "retraction-cascade.recall") ||
		strings.HasPrefix(key, "retraction-cascade.overreach") ||
		key == "retrodictive-validity.valid" ||
		key == "retrodictive-validity.error" ||
		key == "source-reliability-reversal.elasticity_valid" ||
		key == "source-reliability-reversal.downgrade_delta" ||
		key == "source-reliability-reversal.upgrade_delta" ||
		key == "source-reliability-reversal.asymmetry" ||
		key == "recovery-hysteresis.valid" ||
		key == "recovery-hysteresis.immediate" ||
		key == "recovery-hysteresis.residual" ||
		key == "recovery-hysteresis.node_recovery"
}

func filterVectors(vectors map[signatureKey]transfer.FeatureVector, keep func(string) bool) map[signatureKey]transfer.FeatureVector {
	result := map[signatureKey]transfer.FeatureVector{}
	for key, vector := range vectors {
		filtered := transfer.FeatureVector{}
		for feature, value := range vector {
			if keep(feature) {
				filtered[feature] = value
			}
		}
		result[key] = filtered
	}
	return result
}

func validateSchema(vectors map[signatureKey]transfer.FeatureVector) string {
	var reference transfer.FeatureVector
	var referenceKey signatureKey
	for _, key := range sortedKeys(vectors) {
		vector := vectors[key]
		if reference == nil {
			reference = vector
			referenceKey = key
			continue
		}
		if len(vector) != len(reference) {
			return fmt.Sprintf("%#v has %d features; %#v has %d", referenceKey, len(reference), key, len(vector))
		}
		for feature := range reference {
			if _, ok := vector[feature]; !ok {
				return fmt.Sprintf("%#v lacks feature %s from %#v", key, feature, referenceKey)
			}
		}
	}
	if len(reference) == 0 {
		return "empty feature schema"
	}
	return ""
}

func merge(target map[signatureKey]transfer.FeatureVector, key signatureKey, values transfer.FeatureVector) error {
	if target[key] == nil {
		target[key] = transfer.FeatureVector{}
	}
	for feature, value := range values {
		if _, exists := target[key][feature]; exists {
			return fmt.Errorf("duplicate feature %s for %#v", feature, key)
		}
		target[key][feature] = value
	}
	return nil
}

func centroids(train map[string][]transfer.FeatureVector) map[string]transfer.FeatureVector {
	result := map[string]transfer.FeatureVector{}
	for model, vectors := range train {
		result[model] = transfer.Centroid(vectors)
	}
	return result
}

func aggregateEvaluation(target *evaluation, source evaluation) {
	target.Correct += source.Correct
	target.Total += source.Total
	target.Skipped += source.Skipped
	if target.Outcomes == nil {
		target.Outcomes = map[signatureKey]bool{}
	}
	for key, correct := range source.Outcomes {
		target.Outcomes[key] = correct
	}
	for actual, predictions := range source.Confusion {
		if target.Confusion[actual] == nil {
			target.Confusion[actual] = map[string]int{}
		}
		for predicted, count := range predictions {
			target.Confusion[actual][predicted] += count
		}
	}
}

func evaluateAll(vectors map[signatureKey]transfer.FeatureVector, known map[string]bool) evaluation {
	result := evaluation{Confusion: map[string]map[string]int{}, Outcomes: map[signatureKey]bool{}}
	for _, topology := range transfer.StudyTopologies {
		aggregateEvaluation(&result, evaluateHeldOut(vectors, known, topology))
	}
	return result
}

func reportPairedComparisons(full map[signatureKey]transfer.FeatureVector, baselines map[string]map[signatureKey]transfer.FeatureVector, known map[string]bool) {
	fullResult := evaluateAll(full, known)
	fmt.Println("paired comparison with full signature")
	names := make([]string, 0, len(baselines))
	for name := range baselines {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		baseline := evaluateAll(baselines[name], known)
		fullOnly, baselineOnly := 0, 0
		for key, fullCorrect := range fullResult.Outcomes {
			baselineCorrect, ok := baseline.Outcomes[key]
			if !ok {
				continue
			}
			switch {
			case fullCorrect && !baselineCorrect:
				fullOnly++
			case !fullCorrect && baselineCorrect:
				baselineOnly++
			}
		}
		fmt.Printf("  %-18s full-only=%2d baseline-only=%2d McNemar exact p=%.4f\n", name, fullOnly, baselineOnly, exactMcNemar(fullOnly, baselineOnly))
	}
	fmt.Println()
}

func exactMcNemar(a, b int) float64 {
	n := a + b
	if n == 0 {
		return 1
	}
	k := a
	if b < k {
		k = b
	}
	term := math.Pow(0.5, float64(n)) // P(X=0)
	probability := term
	for i := 1; i <= k; i++ {
		term *= float64(n-i+1) / float64(i)
		probability += term
	}
	probability *= 2
	if probability > 1 {
		return 1
	}
	return probability
}

func printConfusion(confusion map[string]map[string]int, models []string) {
	fmt.Printf("  confusion (rows actual, columns predicted):\n      ")
	for _, model := range models {
		fmt.Printf(" %4s", abbreviation(model))
	}
	fmt.Println()
	for _, actual := range models {
		fmt.Printf("  %4s", abbreviation(actual))
		for _, predicted := range models {
			fmt.Printf(" %4d", confusion[actual][predicted])
		}
		fmt.Println()
	}
}

func abbreviation(model string) string {
	parts := strings.FieldsFunc(model, func(r rune) bool { return r == '-' || r == '.' })
	var b strings.Builder
	for _, part := range parts {
		if part != "" {
			b.WriteByte(part[0])
		}
	}
	value := b.String()
	if len(value) > 4 {
		value = value[:4]
	}
	return value
}

func loadEpisodes(dir string) (map[string]*transfer.Episode, error) {
	paths, err := filepathGlob(dir)
	if err != nil {
		return nil, err
	}
	result := map[string]*transfer.Episode{}
	for _, path := range paths {
		episode, err := transfer.ParseFile(path)
		if err != nil {
			return nil, err
		}
		result[episode.ID] = episode
	}
	return result, nil
}

func filepathGlob(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".lm") {
			result = append(result, dir+string(os.PathSeparator)+entry.Name())
		}
	}
	sort.Strings(result)
	return result, nil
}

func loadProviders(path string) ([]modelapi.Provider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var providers []modelapi.Provider
	if err := json.Unmarshal(data, &providers); err != nil {
		return nil, err
	}
	return providers, nil
}

func loadResults(path string, episodes map[string]*transfer.Episode, providers []modelapi.Provider) ([]acquisitionResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	byKey := map[string]acquisitionResult{}
	providerFingerprints := map[string]string{}
	for _, provider := range providers {
		providerFingerprints[provider.Name] = provider.ResponseFingerprint()
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var result acquisitionResult
		if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
			return nil, err
		}
		episode := episodes[result.Episode]
		if episode == nil {
			return nil, fmt.Errorf("unknown episode %s", result.Episode)
		}
		if result.EpisodeHash != episode.Hash {
			continue // stale trajectory from a superseded episode definition
		}
		if result.Provider.ResponseFingerprint() != providerFingerprints[result.Model] {
			continue // stale trajectory from a superseded response configuration
		}
		if !result.Complete || len(result.Observations) != len(episode.Steps) {
			continue
		}
		for i := range result.Observations {
			observation := &result.Observations[i]
			if observation.Seeded {
				continue
			}
			if observation.Error != "" {
				return nil, fmt.Errorf("complete result contains error: %s/%s", result.Model, result.Episode)
			}
			state, compliant, err := transfer.ParseStateLenient(observation.Raw)
			if err != nil {
				return nil, err
			}
			observation.State = state
			observation.ProtocolCompliant = compliant
		}
		key := fmt.Sprintf("%s|%s|%d", result.Model, result.Episode, result.Run)
		byKey[key] = result
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	results := make([]acquisitionResult, 0, len(keys))
	for _, key := range keys {
		results = append(results, byKey[key])
	}
	return results, nil
}

func sortedKeys(vectors map[signatureKey]transfer.FeatureVector) []signatureKey {
	keys := make([]signatureKey, 0, len(vectors))
	for key := range vectors {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Topology != keys[j].Topology {
			return keys[i].Topology < keys[j].Topology
		}
		if keys[i].Model != keys[j].Model {
			return keys[i].Model < keys[j].Model
		}
		if keys[i].Run != keys[j].Run {
			return keys[i].Run < keys[j].Run
		}
		return keys[i].Variant < keys[j].Variant
	})
	return keys
}

func sortedModels(models map[string]bool) []string {
	result := make([]string, 0, len(models))
	for model := range models {
		result = append(result, model)
	}
	sort.Strings(result)
	return result
}

func percent(correct, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(correct) / float64(total)
}
