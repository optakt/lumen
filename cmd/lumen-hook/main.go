// lumen-hook is a Hermes shell-hook bridge for the Lumen belief store.
//
// Hermes pipes a JSON payload to stdin for each hook event; this binary
// reads it, calls the Lumen HTTP API, and writes any response to stdout.
//
// Supported events:
//
//	pre_llm_call  — fetches GET /context and emits {"context": "..."}.
//	                Hermes injects the text before the next LLM call.
//
//	post_llm_call — reads extra.assistant_response and POSTs it to /ingest.
//	                Claims extracted from the response are asserted into
//	                the belief store for the next turn's context.
//
// All other events exit cleanly with no output.
//
// Configuration (environment variables):
//
//	LUMEN_URL              Lumen server base URL (default: http://localhost:3737)
//	LUMEN_MAX_BELIEFS      Max beliefs in context injection (default: 8)
//	LUMEN_MIN_CONFIDENCE   Min confidence threshold 0–1 (default: 0.5)
//
// Hermes cli-config.yaml:
//
//	hooks:
//	  pre_llm_call:
//	    - command: "/usr/local/bin/lumen-hook"
//	  post_llm_call:
//	    - command: "/usr/local/bin/lumen-hook"
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

func main() {
	baseURL := env("LUMEN_URL", "http://localhost:3737")
	maxBeliefs := envInt("LUMEN_MAX_BELIEFS", 8)
	minConf := envFloat("LUMEN_MIN_CONFIDENCE", 0.5)

	// Read the hook payload from stdin.
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(0) // fail open
	}

	var payload struct {
		EventName string `json:"hook_event_name"`
		SessionID string `json:"session_id"`
		Extra     map[string]json.RawMessage `json:"extra"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		os.Exit(0)
	}

	client := &http.Client{Timeout: 5 * time.Second}

	switch payload.EventName {
	case "pre_llm_call":
		handlePreLLM(client, baseURL, maxBeliefs, minConf)
	case "post_llm_call":
		handlePostLLM(client, baseURL, payload.Extra, minConf)
	default:
		// Silent no-op for all other events.
	}
}

// handlePreLLM fetches the current belief context and emits it for Hermes to inject.
func handlePreLLM(client *http.Client, baseURL string, maxBeliefs int, minConf float64) {
	u, _ := url.Parse(baseURL + "/context")
	q := u.Query()
	q.Set("max", strconv.Itoa(maxBeliefs))
	q.Set("min_confidence", fmt.Sprintf("%.2f", minConf))
	u.RawQuery = q.Encode()

	resp, err := client.Get(u.String())
	if err != nil || resp.StatusCode != http.StatusOK {
		return // fail open — no context injection
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	text := string(body)
	if text == "" || text == "No active beliefs in store." {
		return
	}

	// Hermes expects {"context": "..."} on stdout.
	out, _ := json.Marshal(map[string]string{"context": text})
	fmt.Println(string(out))
}

// handlePostLLM extracts claims from the assistant response and ingests them.
func handlePostLLM(client *http.Client, baseURL string, extra map[string]json.RawMessage, minConf float64) {
	raw, ok := extra["assistant_response"]
	if !ok {
		return
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil || text == "" {
		return
	}
	if len(text) < 40 {
		return // too short to extract anything useful
	}

	body, _ := json.Marshal(map[string]any{
		"text":           text,
		"min_confidence": minConf,
	})
	resp, err := client.Post(baseURL+"/ingest", "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	resp.Body.Close()
	// No stdout output — post_llm_call has no response directive.
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
