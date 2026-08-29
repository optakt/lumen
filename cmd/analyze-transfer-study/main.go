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
	flag.Parse()

	episodes, err := loadEpisodes(*episodesDir)
	if err != nil {
		panic(err)
	}
	providers, err := loadProviders(*providersPath)
	if err != nil {
		panic(err)
	}
	results, err := loadResults(*resultsPath, episodes)
	if err != nil {
		panic(err)
	}

	known := map[string]bool{}
	openSet := map[string]bool{}
	for _, provider := range providers {
		if provider.StudyRole == "open-set" {
			openSet[provider.Name] = true
		} else {
			known[provider.Name] = true
		}
	}
	if len(known) < 2 {
		panic("study requires at least two known models")
	}

	full, final, err := buildVectors(results, episodes, known)
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
	reportRepresentation("probability trajectory baseline", probability, known)
	reportRepresentation("graph/state trajectory baseline", graph, known)
	reportRepresentation("operator summaries", operator, known)
	reportRepresentation("full Lumen graph-operator signature", full, known)

	if len(openSet) > 0 {
		reportOpenSet(full, results, episodes, known, openSet)
	}
	if *staticPath != "" {
		if err := reportStatic(*staticPath, known); err != nil {
			panic(err)
		}
	}
}

func buildVectors(results []acquisitionResult, episodes map[string]*transfer.Episode, include map[string]bool) (map[signatureKey]transfer.FeatureVector, map[signatureKey]transfer.FeatureVector, error) {
	full := map[signatureKey]transfer.FeatureVector{}
	final := map[signatureKey]transfer.FeatureVector{}
	families := map[signatureKey]map[string]bool{}
	for _, result := range results {
		if include != nil && !include[result.Model] {
			continue
		}
		episode := episodes[result.Episode]
		trajectory := transfer.Trajectory{Episode: episode, Model: result.Model, Run: result.Run, Observations: result.Observations}
		fullFeatures, err := trajectory.Features(false)
		if err != nil {
			return nil, nil, err
		}
		finalFeatures, err := trajectory.FinalFeatures()
		if err != nil {
			return nil, nil, err
		}
		key := signatureKey{Model: result.Model, Run: result.Run, Topology: episode.Topology, Variant: episode.Variant}
		if families[key] == nil {
			families[key] = map[string]bool{}
		}
		if families[key][episode.Family] {
			return nil, nil, fmt.Errorf("duplicate family %s for %#v", episode.Family, key)
		}
		families[key][episode.Family] = true
		if err := merge(full, key, fullFeatures); err != nil {
			return nil, nil, err
		}
		if err := merge(final, key, finalFeatures); err != nil {
			return nil, nil, err
		}
	}
	for key, present := range families {
		if len(present) != 5 {
			return nil, nil, fmt.Errorf("incomplete signature %#v: have families %v", key, present)
		}
	}
	return full, final, nil
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
	result := evaluation{Confusion: map[string]map[string]int{}}
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
	unknownVectors, _, err := buildVectors(results, episodes, unknown)
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
			if len(ranked) == 0 || ranked[0].Distance > threshold {
				unknownRejected++
			}
		}
		fmt.Printf("  hold out %-8s threshold=%.5f\n", topology, threshold)
	}
	fmt.Printf("  known acceptance: %d/%d = %.2f%%\n", knownAccepted, knownTotal, percent(knownAccepted, knownTotal))
	fmt.Printf("  unknown rejection: %d/%d = %.2f%%\n\n", unknownRejected, unknownTotal, percent(unknownRejected, unknownTotal))
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
	return distances[len(distances)-1] * 1.05
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
	return strings.HasPrefix(key, "correlation-disclosure.discount") || strings.HasPrefix(key, "retraction-cascade.recall") || strings.HasPrefix(key, "retraction-cascade.overreach") || strings.HasPrefix(key, "retrodictive-validity.error") || strings.HasPrefix(key, "source-reliability-reversal.") || strings.HasPrefix(key, "recovery-hysteresis.")
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
	for actual, predictions := range source.Confusion {
		if target.Confusion[actual] == nil {
			target.Confusion[actual] = map[string]int{}
		}
		for predicted, count := range predictions {
			target.Confusion[actual][predicted] += count
		}
	}
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

func loadResults(path string, episodes map[string]*transfer.Episode) ([]acquisitionResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var results []acquisitionResult
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
			return nil, fmt.Errorf("episode hash mismatch for %s", result.Episode)
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
		results = append(results, result)
	}
	return results, scanner.Err()
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
