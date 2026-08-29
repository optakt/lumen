// epistemic-transfer runs and analyzes Lumen belief-graph intervention
// episodes for black-box model identification.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
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

type runResult struct {
	Episode      string                 `json:"episode"`
	Family       string                 `json:"family"`
	Variant      string                 `json:"variant"`
	Model        string                 `json:"model"`
	Run          int                    `json:"run"`
	Observations []transfer.Observation `json:"observations"`
	Timestamp    string                 `json:"timestamp"`
}

type signatureKey struct {
	model   string
	run     int
	variant string
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
	completed, err := completedRuns(resultsPath)
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
			jobs := make([]struct {
				episode *transfer.Episode
				run     int
			}, 0, len(episodes)*runs)
			for run := 1; run <= runs; run++ {
				for _, episode := range episodes {
					key := runKey(provider.Name, episode.ID, run)
					if !completed[key] {
						jobs = append(jobs, struct {
							episode *transfer.Episode
							run     int
						}{episode, run})
					}
				}
			}
			rng.Shuffle(len(jobs), func(i, j int) { jobs[i], jobs[j] = jobs[j], jobs[i] })

			for _, job := range jobs {
				log.Printf("[%s run=%d] %s", provider.Name, job.run, job.episode.ID)
				result, err := runEpisode(client, provider, apiKey, job.episode, job.run)
				if err != nil {
					log.Printf("[%s run=%d] %s ERROR: %v", provider.Name, job.run, job.episode.ID, err)
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					continue
				}
				line, err := json.Marshal(result)
				if err != nil {
					continue
				}
				writeMu.Lock()
				_, err = out.Write(append(line, '\n'))
				writeMu.Unlock()
				if err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					return
				}
			}
		}(providerIndex, provider, apiKey)
	}
	wg.Wait()
	return firstErr
}

func runEpisode(client *modelapi.Client, provider modelapi.Provider, apiKey string, episode *transfer.Episode, run int) (runResult, error) {
	result := runResult{
		Episode:   episode.ID,
		Family:    episode.Family,
		Variant:   episode.Variant,
		Model:     provider.Name,
		Run:       run,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
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
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		if lastErr != nil {
			return runResult{}, fmt.Errorf("step %s: %w; raw=%q", step.ID, lastErr, truncate(raw, 300))
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
	return result, nil
}

func analyze(path string, episodes []*transfer.Episode) error {
	results, err := loadResults(path)
	if err != nil {
		return err
	}
	episodeByID := map[string]*transfer.Episode{}
	for _, episode := range episodes {
		episodeByID[episode.ID] = episode
	}

	dynamic := map[signatureKey]transfer.FeatureVector{}
	static := map[signatureKey]transfer.FeatureVector{}
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
		sta, err := trajectory.Features(true)
		if err != nil {
			return err
		}
		key := signatureKey{result.Model, result.Run, result.Variant}
		mergeFeatures(dynamic, key, dyn)
		mergeFeatures(static, key, sta)
	}

	variants := []string{"a", "b"}
	fmt.Println("EPISTEMIC SYSTEM IDENTIFICATION PILOT")
	fmt.Println(strings.Repeat("=", 54))
	fmt.Printf("Complete trajectories: %d\n\n", len(results))
	for _, heldOut := range variants {
		train := "a"
		if heldOut == "a" {
			train = "b"
		}
		fmt.Printf("Train variant %s → held-out variant %s\n", train, heldOut)
		printEvaluation("static first-state baseline", evaluate(static, train, heldOut))
		printEvaluation("full intervention trajectory", evaluate(dynamic, train, heldOut))
		fmt.Println("  Ablations:")
		printScore("without protocol compliance", evaluate(filterVectors(dynamic, func(key string) bool {
			return !strings.Contains(key, "protocol_compliance")
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
	rows    []string
}

func evaluate(vectors map[signatureKey]transfer.FeatureVector, trainVariant, testVariant string) evaluation {
	byModel := map[string][]transfer.FeatureVector{}
	for key, vector := range vectors {
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
		ranked := transfer.RankedDistances(vectors[key], centroids)
		if len(ranked) == 0 {
			continue
		}
		result.total++
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
	accuracy := 0.0
	if result.total > 0 {
		accuracy = float64(result.correct) / float64(result.total)
	}
	fmt.Printf("  %-30s %d/%d = %.1f%%\n", name, result.correct, result.total, accuracy*100)
	for _, row := range result.rows {
		fmt.Println(row)
	}
}

func printScore(name string, result evaluation) {
	accuracy := 0.0
	if result.total > 0 {
		accuracy = float64(result.correct) / float64(result.total)
	}
	fmt.Printf("    %-28s %d/%d = %.1f%%\n", name, result.correct, result.total, accuracy*100)
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
	return strings.Contains(key, "residual") ||
		strings.HasSuffix(key, ".state_match") ||
		strings.HasSuffix(key, ".action_match") ||
		strings.HasSuffix(key, ".support_jaccard") ||
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

func mergeFeatures(target map[signatureKey]transfer.FeatureVector, key signatureKey, values transfer.FeatureVector) {
	if target[key] == nil {
		target[key] = transfer.FeatureVector{}
	}
	for feature, value := range values {
		target[key][feature] = value
	}
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

func completedRuns(path string) (map[string]bool, error) {
	completed := map[string]bool{}
	results, err := loadResults(path)
	if os.IsNotExist(err) {
		return completed, nil
	}
	if err != nil {
		return nil, err
	}
	for _, result := range results {
		completed[runKey(result.Model, result.Episode, result.Run)] = true
	}
	return completed, nil
}

func loadResults(path string) ([]runResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var results []runResult
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var result runResult
		if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, scanner.Err()
}

func runKey(model, episode string, run int) string {
	return fmt.Sprintf("%s|%s|%d", model, episode, run)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
