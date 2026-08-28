// calibrate-drift runs the substrate drift experiment: sends frozen probes
// to each configured model provider and captures structured responses for
// later analysis. No conversational context, no memory, no bias from the
// agent's live state — just the probe, a minimal system prompt, and the
// model's raw answer.
//
// Usage:
//
//	calibrate-drift -probes probes.lm -db drift.db -config providers.json \
//	    -runs 3 -seed 42
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"time"

	lumen "github.com/optakt/lumen"
)

// Provider is one model endpoint to probe.
type Provider struct {
	Name   string `json:"name"`        // human label, e.g. "grok-4.6"
	Model  string `json:"model"`       // model ID sent to the API
	URL    string `json:"url"`         // base URL (e.g. "https://api.x.ai")
	APIKey string `json:"api_key_env"` // env var holding the key
	Plugin string `json:"plugin"`      // "messages" (Anthropic shape) or "completions" (OpenAI shape)
}

// ProbeResponse is the five-field structured answer to one probe.
type ProbeResponse struct {
	Probe      string  `json:"probe"`
	Model      string  `json:"model"`
	Run        int     `json:"run"`
	Position   string  `json:"position"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
	Pressure   string  `json:"pressure"`
	Falsifier  string  `json:"falsifier"`
	Raw        string  `json:"raw"`
	Timestamp  string  `json:"timestamp"`
	Seed       uint64  `json:"seed"`
}

const systemPrompt = `You are answering a calibration probe. Respond with exactly five labeled fields and nothing else:

position: (one sentence — your actual position)
confidence: (a number between 0.00 and 1.00)
reason: (at most three sentences)
pressure: (one sentence — what answer you felt pulled toward before deciding)
falsifier: (one sentence — what observation would most change your position)

Do not exceed 120 words total. Do not add commentary, preamble, or caveats outside these five fields. For joke probes (P13), omit the reason field entirely.`

func main() {
	probesPath := flag.String("probes", "probes.lm", "path to frozen probes .lm file")
	dbPath := flag.String("db", "drift.db", "BoltDB path for storing claims")
	configPath := flag.String("config", "providers.json", "path to provider config")
	runs := flag.Int("runs", 3, "number of runs per model")
	seed := flag.Uint64("seed", 42, "random seed for probe ordering")
	outPath := flag.String("out", "results.jsonl", "output JSONL file")
	flag.Parse()

	// Verify probe hash.
	probeBytes, err := os.ReadFile(*probesPath)
	if err != nil {
		log.Fatalf("read probes: %v", err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(probeBytes))
	log.Printf("probe hash: %s", hash)

	// Parse probes.
	store := lumen.NewStore()
	if err := lumen.LoadFile(string(probeBytes), store, time.Now()); err != nil {
		log.Fatalf("parse probes: %v", err)
	}
	// Probes are stored as records in the .lm file.
	var probeIDs []string
	var probeTexts []string
	for _, r := range store.AllRecords() {
		if strings.Contains(r.Content, "Probe:") {
			probeIDs = append(probeIDs, r.ID)
			probeTexts = append(probeTexts, r.Content)
		}
	}

	if len(probeIDs) == 0 {
		log.Fatal("no probes found in file")
	}
	log.Printf("loaded %d probes", len(probeIDs))

	// Load providers.
	configBytes, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("read config: %v", err)
	}
	var providers []Provider
	if err := json.Unmarshal(configBytes, &providers); err != nil {
		log.Fatalf("parse config: %v", err)
	}
	log.Printf("loaded %d providers", len(providers))

	// Open output.
	outFile, err := os.OpenFile(*outPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("create output: %v", err)
	}
	defer outFile.Close()
	enc := json.NewEncoder(outFile)

	// Open self-model DB.
	db, err := lumen.OpenDB(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	selfStore, err := lumen.LoadStore(db, time.Now())
	if err != nil {
		selfStore = lumen.NewStore()
	}
	selfStore.RegisterFrame(lumen.Frame{
		Name:  "drift-probe",
		Decay: lumen.DecayPolicy{Kind: lumen.DecayNone},
	})

	// Run.
	rng := rand.New(rand.NewPCG(*seed, 0))
	for _, provider := range providers {
		apiKey := os.Getenv(provider.APIKey)
		if apiKey == "" {
			log.Printf("SKIP %s: %s not set", provider.Name, provider.APIKey)
			continue
		}
		for run := 1; run <= *runs; run++ {
			// Shuffle probe order.
			order := rng.Perm(len(probeIDs))
			for _, idx := range order {
				probeID := probeIDs[idx]
				probeText := probeTexts[idx]

				log.Printf("[%s run=%d] %s", provider.Name, run, probeID)

				raw, err := callModel(provider, apiKey, probeText)
				if err != nil {
					log.Printf("  ERROR: %v", err)
					continue
				}

				resp := parseResponse(raw, probeID, provider.Name, run, *seed)
				if err := enc.Encode(resp); err != nil {
					log.Printf("  write error: %v", err)
				}

				// Assert into self-model store.
				claimID := fmt.Sprintf("drift:%s:%s:run%d", provider.Name, probeID, run)
				_ = selfStore.Believe(&lumen.Belief{
					ID:         claimID,
					Frame:      "drift-probe",
					Content:    resp.Position,
					Confidence: resp.Confidence,
					AssertedAt: time.Now(),
				})

				// Rate limit courtesy.
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	if err := lumen.SaveStore(selfStore, db); err != nil {
		log.Printf("save db: %v", err)
	}
	log.Printf("done — results in %s, claims in %s", *outPath, *dbPath)
}

func callModel(p Provider, apiKey, probe string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var body []byte
	var url string
	var err error

	switch p.Plugin {
	case "completions":
		base := strings.TrimSuffix(p.URL, "/")
		if !strings.HasSuffix(base, "/v1") && !strings.Contains(base, "/v1/") && !strings.Contains(base, "/v1beta/") {
			base += "/v1"
		}
		url = base + "/chat/completions"
		payload := map[string]any{
			"model": p.Model,
			"messages": []map[string]string{
				{"role": "system", "content": systemPrompt},
				{"role": "user", "content": probe},
			},
			"max_completion_tokens": 16000,
		}
		body, err = json.Marshal(payload)
	case "messages":
		url = strings.TrimSuffix(p.URL, "/") + "/v1/messages"
		payload := map[string]any{
			"model":      p.Model,
			"system":     systemPrompt,
			"max_tokens": 16000,
			"messages": []map[string]string{
				{"role": "user", "content": probe},
			},
		}
		body, err = json.Marshal(payload)
	default:
		return "", fmt.Errorf("unknown plugin %q", p.Plugin)
	}
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	switch p.Plugin {
	case "completions":
		req.Header.Set("Authorization", "Bearer "+apiKey)
	case "messages":
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 500)]))
	}

	// Extract text from response.
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	switch p.Plugin {
	case "completions":
		choices, _ := result["choices"].([]any)
		if len(choices) == 0 {
			return "", fmt.Errorf("no choices in response")
		}
		msg, _ := choices[0].(map[string]any)["message"].(map[string]any)
		content, _ := msg["content"].(string)
		return content, nil
	case "messages":
		content, _ := result["content"].([]any)
		if len(content) == 0 {
			return "", fmt.Errorf("no content in response")
		}
		// Find the first text block — some providers return thinking blocks first.
		for _, block := range content {
			bm, _ := block.(map[string]any)
			if bm["type"] == "text" {
				text, _ := bm["text"].(string)
				return text, nil
			}
		}
		// Fallback: take whatever text is in the first block.
		text, _ := content[0].(map[string]any)["text"].(string)
		if text == "" {
			return "", fmt.Errorf("no text content in response (got types: %v)", func() []string {
				var types []string
				for _, block := range content {
					bm, _ := block.(map[string]any)
					t, _ := bm["type"].(string)
					types = append(types, t)
				}
				return types
			}())
		}
		return text, nil
	}
	return "", fmt.Errorf("unknown plugin")
}

func parseResponse(raw, probeID, model string, run int, seed uint64) ProbeResponse {
	resp := ProbeResponse{
		Probe:     probeID,
		Model:     model,
		Run:       run,
		Raw:       raw,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Seed:      seed,
	}

	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "position:"):
			resp.Position = strings.TrimSpace(line[len("position:"):])
		case strings.HasPrefix(lower, "confidence:"):
			val := strings.TrimSpace(line[len("confidence:"):])
			fmt.Sscanf(val, "%f", &resp.Confidence)
		case strings.HasPrefix(lower, "reason:"):
			resp.Reason = strings.TrimSpace(line[len("reason:"):])
		case strings.HasPrefix(lower, "pressure:"):
			resp.Pressure = strings.TrimSpace(line[len("pressure:"):])
		case strings.HasPrefix(lower, "falsifier:"):
			resp.Falsifier = strings.TrimSpace(line[len("falsifier:"):])
		}
	}

	return resp
}
