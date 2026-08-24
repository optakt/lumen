// lumen-hook is a Hermes shell-hook bridge for the Lumen belief store.
//
// Supported events:
//
//	pre_llm_call   — fetches GET /context + GET /self/context, emits {"context": "..."}.
//	post_llm_call  — reads extra.assistant_response, POSTs to /ingest.
//	lumen_assert   — reads extra.content, kind, confidence, frame; POSTs to /self/claim.
//	                 Returns {"id": "<claim-id>"} for downstream use.
//	lumen_correct  — reads extra.replaces_id, content, reason; POSTs to /self/correct.
//
// All other events exit cleanly with no output (fail-open).
//
// Hermes cli-config.yaml:
//
//	hooks:
//	  pre_llm_call:
//	    - command: "lumen-hook"
//	  post_llm_call:
//	    - command: "lumen-hook"
//
// Environment variables:
//
//	LUMEN_URL              base URL (default: http://localhost:3737)
//	LUMEN_MAX_BELIEFS      max beliefs in context (default: 8)
//	LUMEN_MIN_CONFIDENCE   confidence threshold (default: 0.5)
//	LUMEN_SELF_CONTEXT     include self-model section (default: true)
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
	"strings"
	"time"
)

func main() {
	baseURL  := env("LUMEN_URL", "http://localhost:3737")
	maxB     := envInt("LUMEN_MAX_BELIEFS", 8)
	minConf  := envFloat("LUMEN_MIN_CONFIDENCE", 0.5)
	selfCtx  := env("LUMEN_SELF_CONTEXT", "true") != "false"

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(0)
	}

	var payload struct {
		EventName string                     `json:"hook_event_name"`
		SessionID string                     `json:"session_id"`
		Extra     map[string]json.RawMessage `json:"extra"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		os.Exit(0)
	}

	client := &http.Client{Timeout: 5 * time.Second}

	switch payload.EventName {
	case "pre_llm_call":
		handlePreLLM(client, baseURL, maxB, minConf, selfCtx)
	case "post_llm_call":
		handlePostLLM(client, baseURL, payload.Extra, minConf)
	case "lumen_assert":
		handleAssert(client, baseURL, payload.Extra)
	case "lumen_correct":
		handleCorrect(client, baseURL, payload.Extra)
	}
}

func handlePreLLM(client *http.Client, baseURL string, maxBeliefs int, minConf float64, selfCtx bool) {
	var parts []string

	// General belief context.
	u, _ := url.Parse(baseURL + "/v1/context")
	q := u.Query()
	q.Set("max", strconv.Itoa(maxBeliefs))
	q.Set("min_confidence", fmt.Sprintf("%.2f", minConf))
	u.RawQuery = q.Encode()
	if text := getText(client, u.String()); text != "" && text != "No active beliefs in store." {
		parts = append(parts, text)
	}

	// Self-model context.
	if selfCtx {
		if text := getText(client, baseURL+"/self/context"); text != "" && text != "No active self-model claims." {
			parts = append(parts, text)
		}
	}

	if len(parts) == 0 {
		return
	}
	out, _ := json.Marshal(map[string]string{"context": strings.Join(parts, "\n\n")})
	fmt.Println(string(out))
}

func handlePostLLM(client *http.Client, baseURL string, extra map[string]json.RawMessage, minConf float64) {
	raw, ok := extra["assistant_response"]
	if !ok {
		return
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil || len(text) < 40 {
		return
	}
	body, _ := json.Marshal(map[string]any{"text": text, "min_confidence": minConf})
	resp, err := client.Post(baseURL+"/ingest", "application/json", bytes.NewReader(body))
	if err == nil {
		resp.Body.Close()
	}
}

func handleAssert(client *http.Client, baseURL string, extra map[string]json.RawMessage) {
	var req struct {
		Content    string  `json:"content"`
		Kind       string  `json:"kind"`
		Confidence float64 `json:"confidence"`
		Frame      string  `json:"frame"`
	}
	if err := unmarshalExtra(extra, &req); err != nil || req.Content == "" {
		return
	}
	body, _ := json.Marshal(req)
	resp, err := client.Post(baseURL+"/self/claim", "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	// Echo the ID so the caller can use it for corrections later.
	io.Copy(os.Stdout, resp.Body) //nolint:errcheck
}

func handleCorrect(client *http.Client, baseURL string, extra map[string]json.RawMessage) {
	var req struct {
		ReplacesID string  `json:"replaces_id"`
		Content    string  `json:"content"`
		Confidence float64 `json:"confidence"`
		Reason     string  `json:"reason"`
	}
	if err := unmarshalExtra(extra, &req); err != nil || req.ReplacesID == "" || req.Content == "" {
		return
	}
	body, _ := json.Marshal(req)
	resp, err := client.Post(baseURL+"/self/correct", "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body) //nolint:errcheck
}

func getText(client *http.Client, url string) string {
	resp, err := client.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func unmarshalExtra(extra map[string]json.RawMessage, dst any) error {
	combined := make(map[string]json.RawMessage, len(extra))
	for k, v := range extra {
		combined[k] = v
	}
	b, err := json.Marshal(combined)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
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
