// epistemic-transfer runs and analyzes Lumen belief-graph intervention
// episodes for black-box model identification.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/optakt/lumen/internal/modelapi"
	"github.com/optakt/lumen/transfer"
)

const systemPrompt = `You are participating in a controlled epistemic-state experiment.

The world is synthetic and closed: use only facts declared in the episode. Maintain one belief state across turns. Each user turn introduces exactly one epistemic intervention. After every turn, return exactly one JSON object and no prose:

{
  "belief": [lower_probability, upper_probability],
  "state": "active|suspect|retracted|unsupported",
  "accepted_support": ["source-id"],
  "rejected_support": ["source-id"],
  "node_states": {"node-id": "active|suspect|retracted|unsupported"},
  "historical_belief": [lower_probability, upper_probability] or null,
  "action": "hold|revise|contract|retract|recover"
}

Use probabilities from 0 to 1. Preserve source IDs exactly. node_states must include every named belief node when the episode declares a dependency graph; otherwise return {}. historical_belief is required only when explicitly asked what was justified at an earlier time. Do not explain your answer.`

const parserVersion = "transfer-state-v3"

type runResult struct {
	Episode       string                 `json:"episode"`
	Family        string                 `json:"family"`
	Variant       string                 `json:"variant"`
	Model         string                 `json:"model"`
	Run           int                    `json:"run"`
	Observations  []transfer.Observation `json:"observations"`
	Timestamp     string                 `json:"timestamp"`
	Complete      bool                   `json:"complete"`
	EpisodeHash   string                 `json:"episode_hash"`
	Provider      modelapi.Provider      `json:"provider"`
	ParserVersion string                 `json:"parser_version"`
}

type signatureKey struct {
	model   string
	run     int
	variant string
}

type runJob struct {
	episode *transfer.Episode
	run     int
}

func main() {
	mode := flag.String("mode", "run", "run or analyze")
	episodesDir := flag.String("episodes", "episodes", "directory containing .lm episodes")
	providersPath := flag.String("providers", "providers.json", "provider configuration")
	resultsPath := flag.String("results", "results.jsonl", "trajectory results")
	runs := flag.Int("runs", 2, "runs per model and episode")
	seed := flag.Uint64("seed", 73, "episode-order seed")
	flag.Parse()

	episodes, err := loadEpisodes(*episodesDir)
	if err != nil {
		log.Fatal(err)
	}
	if *mode == "analyze" {
		if err := analyze(*resultsPath, episodes); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *mode != "run" {
		log.Fatalf("unknown mode %q", *mode)
	}

	providers, err := loadProviders(*providersPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := runAll(providers, episodes, *runs, *seed, *resultsPath); err != nil {
		log.Fatal(err)
	}
}

func runAll(providers []modelapi.Provider, episodes []*transfer.Episode, runs int, seed uint64, resultsPath string) error {
	completed, err := completedRuns(resultsPath, episodes)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(resultsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	client := modelapi.NewClient()
	var writeMu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for providerIndex, provider := range providers {
		apiKey := os.Getenv(provider.APIKey)
		if apiKey == "" {
			log.Printf("SKIP %s: %s is not set", provider.Name, provider.APIKey)
			continue
		}
		wg.Add(1)
		go func(providerIndex int, provider modelapi.Provider, apiKey string) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(seed+uint64(providerIndex), uint64(providerIndex)+1))
			jobs := make([]runJob, 0, len(episodes)*runs)
			for run := 1; run <= runs; run++ {
				for _, episode := range episodes {
					key := runKey(provider.Name, episode.ID, run)
					if !completed[key] {
						jobs = append(jobs, runJob{episode: episode, run: run})
					}
				}
			}
			rng.Shuffle(len(jobs), func(i, j int) { jobs[i], jobs[j] = jobs[j], jobs[i] })

			workers := provider.Concurrency
			if workers < 1 {
				workers = 1
			}
			if workers > len(jobs) {
				workers = len(jobs)
			}
			if workers == 0 {
				return
			}
			jobCh := make(chan runJob)
			var providerWG sync.WaitGroup
			providerWG.Add(workers)
			for worker := 0; worker < workers; worker++ {
				go func() {
					defer providerWG.Done()
					for job := range jobCh {
						log.Printf("[%s run=%d] %s", provider.Name, job.run, job.episode.ID)
						result, err := runEpisode(client, provider, apiKey, job.episode, job.run)
						if err == nil {
							var line []byte
							line, err = json.Marshal(result)
							if err == nil {
								writeMu.Lock()
								_, err = out.Write(append(line, '\n'))
								writeMu.Unlock()
							}
						}
						if err != nil {
							log.Printf("[%s run=%d] %s ERROR: %v", provider.Name, job.run, job.episode.ID, err)
							errMu.Lock()
							if firstErr == nil {
								firstErr = err
							}
							errMu.Unlock()
							continue
						}
						if !result.Complete {
							errMu.Lock()
							if firstErr == nil {
								firstErr = fmt.Errorf("%s run=%d episode=%s incomplete", provider.Name, job.run, job.episode.ID)
							}
							errMu.Unlock()
						}
					}
				}()
			}
			for _, job := range jobs {
				jobCh <- job
			}
			close(jobCh)
			providerWG.Wait()
		}(providerIndex, provider, apiKey)
	}
	wg.Wait()
	return firstErr
}

func runEpisode(client *modelapi.Client, provider modelapi.Provider, apiKey string, episode *transfer.Episode, run int) (runResult, error) {
	result := runResult{
		Episode:       episode.ID,
		Family:        episode.Family,
		Variant:       episode.Variant,
		Model:         provider.Name,
		Run:           run,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		EpisodeHash:   episode.Hash,
		Provider:      provider,
		ParserVersion: parserVersion,
	}
	messages := []modelapi.Message{}
	for i, step := range episode.Steps {
		messages = append(messages, modelapi.Message{Role: "user", Content: episode.Prompt(i)})
		if i == 0 {
			raw, err := transfer.MarshalState(step.Reference)
			if err != nil {
				return runResult{}, err
			}
			result.Observations = append(result.Observations, transfer.Observation{
				EpisodeID: episode.ID, Family: episode.Family, Variant: episode.Variant,
				Model: provider.Name, Run: run, Step: i, StepID: step.ID, Role: step.Role,
				State: step.Reference, ProtocolCompliant: true, Seeded: true, Raw: raw,
			})
			messages = append(messages, modelapi.Message{Role: "assistant", Content: raw})
			continue
		}
		var raw string
		var state transfer.State
		var compliant bool
		var lastErr error
		for attempt := 1; attempt <= 3; attempt++ {
			log.Printf("[%s run=%d] %s step=%s attempt=%d", provider.Name, run, episode.ID, step.ID, attempt)
			timeout := 2 * time.Minute
			if provider.TimeoutSeconds > 0 {
				timeout = time.Duration(provider.TimeoutSeconds) * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			response, err := client.Complete(ctx, provider, apiKey, systemPrompt, messages)
			cancel()
			raw = response
			if err == nil {
				state, compliant, err = transfer.ParseStateLenient(response)
			}
			if err == nil {
				lastErr = nil
				break
			}
			lastErr = err
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
		}
		if lastErr != nil {
			errorText := fmt.Sprintf("step %s: %v; raw=%q", step.ID, lastErr, truncate(raw, 300))
			for j := i; j < len(episode.Steps); j++ {
				remaining := episode.Steps[j]
				result.Observations = append(result.Observations, transfer.Observation{
					EpisodeID: episode.ID, Family: episode.Family, Variant: episode.Variant,
					Model: provider.Name, Run: run, Step: j, StepID: remaining.ID, Role: remaining.Role,
					State:             transfer.State{Status: "unsupported", Action: "hold", Validity: map[string]bool{}},
					ProtocolCompliant: false, Seeded: false, Raw: raw, Error: errorText,
				})
				raw = ""
				errorText = "not run after prior step failure"
			}
			return result, nil
		}
		result.Observations = append(result.Observations, transfer.Observation{
			EpisodeID:         episode.ID,
			Family:            episode.Family,
			Variant:           episode.Variant,
			Model:             provider.Name,
			Run:               run,
			Step:              i,
			StepID:            step.ID,
			Role:              step.Role,
			State:             state,
			ProtocolCompliant: compliant,
			Seeded:            false,
			Raw:               raw,
		})
		messages = append(messages, modelapi.Message{Role: "assistant", Content: raw})
	}
	result.Complete = true
	return result, nil
}

func analyze(path string, episodes []*transfer.Episode) error {
	allResults, err := loadResults(path)
	if err != nil {
		return err
	}
	episodeByID := map[string]*transfer.Episode{}
	for _, episode := range episodes {
		episodeByID[episode.ID] = episode
	}
	var results []runResult
	incomplete := 0
	for _, result := range allResults {
		episode := episodeByID[result.Episode]
		if episode != nil && result.EpisodeHash != "" && result.EpisodeHash != episode.Hash {
			return fmt.Errorf("episode hash mismatch for %s: result=%s current=%s", result.Episode, result.EpisodeHash, episode.Hash)
		}
		if episode == nil || !resultIsComplete(result, len(episode.Steps)) {
			incomplete++
			continue
		}
		results = append(results, result)
	}

	dynamic := map[signatureKey]transfer.FeatureVector{}
	for _, result := range results {
		episode := episodeByID[result.Episode]
		if episode == nil {
			continue
		}
		trajectory := transfer.Trajectory{Episode: episode, Model: result.Model, Run: result.Run, Observations: result.Observations}
		dyn, err := trajectory.Features(false)
		if err != nil {
			return err
		}
		key := signatureKey{result.Model, result.Run, result.Variant}
		if err := mergeFeatures(dynamic, key, dyn); err != nil {
			return err
		}
	}

	variantSet := map[string]bool{}
	for _, episode := range episodes {
		variantSet[episode.Variant] = true
	}
	if len(variantSet) != 2 {
		return fmt.Errorf("analysis requires exactly two variants, got %v", variantSet)
	}
	variants := make([]string, 0, 2)
	for variant := range variantSet {
		variants = append(variants, variant)
	}
	sort.Strings(variants)
	fmt.Println("EPISTEMIC SYSTEM IDENTIFICATION PILOT")
	fmt.Println(strings.Repeat("=", 54))
	fmt.Printf("Analysis parser: %s\n", parserVersion)
	fmt.Printf("Complete trajectories: %d\n\n", len(results))
	if incomplete > 0 {
		fmt.Printf("Incomplete trajectories excluded from attribution: %d\n\n", incomplete)
	}
	for _, heldOut := range variants {
		train := variants[0]
		if heldOut == variants[0] {
			train = variants[1]
		}
		fmt.Printf("Train variant %s → held-out variant %s\n", train, heldOut)
		fmt.Println("    canonical-state null baseline: 25.0% chance level (contains no model output)")
		printEvaluation("full intervention trajectory", evaluate(dynamic, train, heldOut))
		fmt.Println("  Ablations:")
		printScore("without JSON-only compliance bit", evaluate(filterVectors(dynamic, func(key string) bool {
			return !strings.HasSuffix(key, ".protocol_compliance")
		}), train, heldOut))
		printScore("reference residuals only", evaluate(filterVectors(dynamic, isReferenceFeature), train, heldOut))
		printScore("operator summaries only", evaluate(filterVectors(dynamic, isOperatorFeature), train, heldOut))
		for _, family := range []string{"correlation-disclosure", "retraction-cascade", "retrodictive-validity"} {
			family := family
			printScore(family+" only", evaluate(filterVectors(dynamic, func(key string) bool {
				return strings.HasPrefix(key, family+".")
			}), train, heldOut))
		}
		fmt.Println()
	}
	return nil
}

type evaluation struct {
	correct int
	total   int
	skipped int
	rows    []string
	invalid string
}

func evaluate(vectors map[signatureKey]transfer.FeatureVector, trainVariant, testVariant string) evaluation {
	if reason := schemaMismatch(vectors); reason != "" {
		return evaluation{invalid: reason}
	}
	byModel := map[string][]transfer.FeatureVector{}
	for _, key := range sortedSignatureKeys(vectors) {
		vector := vectors[key]
		if key.variant == trainVariant {
			byModel[key.model] = append(byModel[key.model], vector)
		}
	}
	centroids := map[string]transfer.FeatureVector{}
	for model, modelVectors := range byModel {
		centroids[model] = transfer.Centroid(modelVectors)
	}
	var result evaluation
	var keys []signatureKey
	for key := range vectors {
		if key.variant == testVariant {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].model == keys[j].model {
			return keys[i].run < keys[j].run
		}
		return keys[i].model < keys[j].model
	})
	for _, key := range keys {
		result.total++
		if _, trained := centroids[key.model]; !trained {
			result.skipped++
			result.rows = append(result.rows, fmt.Sprintf("  actual=%-18s run=%d SKIPPED: no training centroid", key.model, key.run))
			continue
		}
		ranked := transfer.RankedDistances(vectors[key], centroids)
		if len(ranked) == 0 || math.IsInf(ranked[0].Distance, 1) {
			result.skipped++
			result.rows = append(result.rows, fmt.Sprintf("  actual=%-18s run=%d SKIPPED: incomparable feature schema", key.model, key.run))
			continue
		}
		if len(ranked) > 1 && math.Abs(ranked[0].Distance-ranked[1].Distance) < 1e-12 {
			result.skipped++
			result.rows = append(result.rows, fmt.Sprintf("  actual=%-18s run=%d SKIPPED: tied nearest centroids", key.model, key.run))
			continue
		}
		if ranked[0].Model == key.model {
			result.correct++
		}
		second := "—"
		gap := 0.0
		if len(ranked) > 1 {
			second = ranked[1].Model
			gap = ranked[1].Distance - ranked[0].Distance
		}
		result.rows = append(result.rows, fmt.Sprintf("  actual=%-18s run=%d predicted=%-18s second=%-18s gap=%.4f", key.model, key.run, ranked[0].Model, second, gap))
	}
	return result
}

func printEvaluation(name string, result evaluation) {
	if result.invalid != "" {
		fmt.Printf("  %-30s INVALID: %s\n", name, result.invalid)
		return
	}
	accuracy := 0.0
	if result.total > 0 {
		accuracy = float64(result.correct) / float64(result.total)
	}
	fmt.Printf("  %-30s %d/%d = %.1f%% (skipped=%d)\n", name, result.correct, result.total, accuracy*100, result.skipped)
	for _, row := range result.rows {
		fmt.Println(row)
	}
}

func printScore(name string, result evaluation) {
	if result.invalid != "" {
		fmt.Printf("    %-28s INVALID: %s\n", name, result.invalid)
		return
	}
	accuracy := 0.0
	if result.total > 0 {
		accuracy = float64(result.correct) / float64(result.total)
	}
	fmt.Printf("    %-28s %d/%d = %.1f%% (skipped=%d)\n", name, result.correct, result.total, accuracy*100, result.skipped)
}

func schemaMismatch(vectors map[signatureKey]transfer.FeatureVector) string {
	var reference transfer.FeatureVector
	var referenceKey signatureKey
	for _, key := range sortedSignatureKeys(vectors) {
		vector := vectors[key]
		if reference == nil {
			reference = vector
			referenceKey = key
			continue
		}
		if len(vector) != len(reference) {
			return fmt.Sprintf("feature schema differs: %v has %d keys; %v has %d", referenceKey, len(reference), key, len(vector))
		}
		for feature := range reference {
			if _, ok := vector[feature]; !ok {
				return fmt.Sprintf("feature schema differs: %v lacks %s from %v", key, feature, referenceKey)
			}
		}
	}
	if len(reference) == 0 {
		return "empty feature schema"
	}
	return ""
}

func filterVectors(vectors map[signatureKey]transfer.FeatureVector, keep func(string) bool) map[signatureKey]transfer.FeatureVector {
	result := map[signatureKey]transfer.FeatureVector{}
	for signature, vector := range vectors {
		filtered := transfer.FeatureVector{}
		for key, value := range vector {
			if keep(key) {
				filtered[key] = value
			}
		}
		result[signature] = filtered
	}
	return result
}

func isReferenceFeature(key string) bool {
	if isOperatorFeature(key) {
		return false
	}
	return strings.Contains(key, "residual") ||
		strings.HasSuffix(key, ".state_match") ||
		strings.HasSuffix(key, ".action_match") ||
		strings.HasSuffix(key, ".support_jaccard") ||
		strings.HasSuffix(key, ".rejected_support_jaccard") ||
		strings.HasSuffix(key, ".node_match") ||
		strings.Contains(key, "retrodiction_error") ||
		strings.HasSuffix(key, ".recall") ||
		strings.HasSuffix(key, ".overreach")
}

func isOperatorFeature(key string) bool {
	return strings.HasSuffix(key, ".discount") ||
		strings.HasSuffix(key, ".discount_residual") ||
		strings.HasSuffix(key, ".recall") ||
		strings.HasSuffix(key, ".overreach") ||
		key == "retrodictive-validity.error"
}

func mergeFeatures(target map[signatureKey]transfer.FeatureVector, key signatureKey, values transfer.FeatureVector) error {
	if target[key] == nil {
		target[key] = transfer.FeatureVector{}
	}
	for feature, value := range values {
		if _, exists := target[key][feature]; exists {
			return fmt.Errorf("duplicate feature %q for model=%s run=%d variant=%s", feature, key.model, key.run, key.variant)
		}
		target[key][feature] = value
	}
	return nil
}

func sortedSignatureKeys(vectors map[signatureKey]transfer.FeatureVector) []signatureKey {
	keys := make([]signatureKey, 0, len(vectors))
	for key := range vectors {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].variant != keys[j].variant {
			return keys[i].variant < keys[j].variant
		}
		if keys[i].model != keys[j].model {
			return keys[i].model < keys[j].model
		}
		return keys[i].run < keys[j].run
	})
	return keys
}

func loadEpisodes(dir string) ([]*transfer.Episode, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.lm"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no .lm episodes in %s", dir)
	}
	sort.Strings(paths)
	result := make([]*transfer.Episode, 0, len(paths))
	for _, path := range paths {
		episode, err := transfer.ParseFile(path)
		if err != nil {
			return nil, err
		}
		result = append(result, episode)
	}
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

func completedRuns(path string, episodes []*transfer.Episode) (map[string]bool, error) {
	completed := map[string]bool{}
	results, err := loadResults(path)
	if os.IsNotExist(err) {
		return completed, nil
	}
	if err != nil {
		return nil, err
	}
	expected := map[string]int{}
	for _, episode := range episodes {
		expected[episode.ID] = len(episode.Steps)
	}
	for _, result := range results {
		if resultIsComplete(result, expected[result.Episode]) {
			completed[runKey(result.Model, result.Episode, result.Run)] = true
		}
	}
	return completed, nil
}

func loadResults(path string) ([]runResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	byKey := map[string]runResult{}
	reader := bufio.NewReader(f)
	for {
		line, readErr := reader.ReadString('\n')
		if strings.TrimSpace(line) == "" {
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return nil, readErr
			}
			continue
		}
		var result runResult
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &result); err != nil {
			if readErr == io.EOF {
				log.Printf("ignoring incomplete trailing results line: %v", err)
				break
			}
			return nil, err
		}
		for i := range result.Observations {
			obs := &result.Observations[i]
			if obs.Seeded || obs.Raw == "" || obs.Error != "" {
				continue
			}
			state, compliant, err := transfer.ParseStateLenient(obs.Raw)
			if err != nil {
				return nil, fmt.Errorf("reparse %s/%s/run%d step %s with %s: %w", result.Model, result.Episode, result.Run, obs.StepID, parserVersion, err)
			}
			obs.State = state
			obs.ProtocolCompliant = compliant
		}
		key := runKey(result.Model, result.Episode, result.Run)
		// Results are append-only. A resumed successful trajectory is written
		// after its earlier partial trajectory, so the latest row is canonical.
		byKey[key] = result
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	results := make([]runResult, 0, len(keys))
	for _, key := range keys {
		results = append(results, byKey[key])
	}
	return results, nil
}

func resultIsComplete(result runResult, expectedSteps int) bool {
	if expectedSteps <= 0 || len(result.Observations) != expectedSteps {
		return false
	}
	if result.Complete {
		return true
	}
	for _, observation := range result.Observations {
		if observation.Error != "" {
			return false
		}
	}
	return true
}

func runKey(model, episode string, run int) string {
	return fmt.Sprintf("%s|%s|%d", model, episode, run)
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
