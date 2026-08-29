// Package modelapi provides the small provider-neutral chat client used by
// Lumen's black-box experiments. It deliberately exposes only messages and
// text responses: provider metadata must not enter the measured trajectory.
package modelapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTPError preserves response status and retry guidance for acquisition loops.
type HTTPError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// Provider describes one OpenAI- or Anthropic-compatible endpoint.
type Provider struct {
	Name           string         `json:"name"`
	Model          string         `json:"model"`
	URL            string         `json:"url"`
	APIKey         string         `json:"api_key_env"`
	Plugin         string         `json:"plugin"` // "messages" or "completions"
	MaxTokens      int            `json:"max_tokens,omitempty"`
	Temperature    *float64       `json:"temperature,omitempty"`
	Seed           *int64         `json:"seed,omitempty"`
	MaxTokensParam string         `json:"max_tokens_param,omitempty"`
	StudyRole      string         `json:"study_role,omitempty"` // "known" or "open-set"; ignored by transport
	TimeoutSeconds int            `json:"timeout_seconds,omitempty"`
	Extra          map[string]any `json:"extra,omitempty"`
	Concurrency    int            `json:"concurrency,omitempty"` // experiment runner hint; ignored by transport
}

// ResponseFingerprint identifies provider fields that can change model output.
// Operational scheduling fields (concurrency, timeout, study role) are excluded.
func (p Provider) ResponseFingerprint() string {
	value := struct {
		Name, Model, URL, APIKey, Plugin, MaxTokensParam string
		MaxTokens                                        int
		Temperature                                      *float64
		Seed                                             *int64
		Extra                                            map[string]any
	}{p.Name, p.Model, p.URL, p.APIKey, p.Plugin, p.MaxTokensParam, p.MaxTokens, p.Temperature, p.Seed, p.Extra}
	data, _ := json.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// Message is one turn in a provider-neutral conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Client calls black-box model endpoints.
type Client struct {
	HTTP      *http.Client
	MaxTokens int
}

// NewClient returns a client with experiment-safe defaults.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 2 * time.Minute},
		MaxTokens: 16000,
	}
}

// Complete sends a conversation and returns the first text response.
func (c *Client) Complete(ctx context.Context, p Provider, apiKey, system string, messages []Message) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("empty API key for %s", p.Name)
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 2 * time.Minute}
	}
	if p.TimeoutSeconds > 0 {
		clone := *httpClient
		clone.Timeout = time.Duration(p.TimeoutSeconds) * time.Second
		httpClient = &clone
	}
	maxTokens := c.MaxTokens
	if p.MaxTokens > 0 {
		maxTokens = p.MaxTokens
	}
	if maxTokens <= 0 {
		maxTokens = 16000
	}

	var endpoint string
	var payload any
	switch p.Plugin {
	case "completions":
		base := strings.TrimSuffix(p.URL, "/")
		if !strings.HasSuffix(base, "/v1") && !strings.HasSuffix(base, "/v1beta") && !strings.Contains(base, "/v1/") && !strings.Contains(base, "/v1beta/") {
			base += "/v1"
		}
		endpoint = base + "/chat/completions"
		providerMessages := make([]Message, 0, len(messages)+1)
		if system != "" {
			providerMessages = append(providerMessages, Message{Role: "system", Content: system})
		}
		providerMessages = append(providerMessages, messages...)
		maxTokensParam := p.MaxTokensParam
		if maxTokensParam == "" {
			maxTokensParam = "max_completion_tokens"
		}
		payload = map[string]any{
			"model":        p.Model,
			"messages":     providerMessages,
			maxTokensParam: maxTokens,
		}
		if p.Temperature != nil {
			payload.(map[string]any)["temperature"] = *p.Temperature
		}
		if p.Seed != nil {
			payload.(map[string]any)["seed"] = *p.Seed
		}
		for key, value := range p.Extra {
			payload.(map[string]any)[key] = value
		}
	case "messages":
		endpoint = strings.TrimSuffix(p.URL, "/") + "/v1/messages"
		payload = map[string]any{
			"model":      p.Model,
			"system":     system,
			"messages":   messages,
			"max_tokens": maxTokens,
		}
		if p.Temperature != nil {
			payload.(map[string]any)["temperature"] = *p.Temperature
		}
		for key, value := range p.Extra {
			payload.(map[string]any)[key] = value
		}
	default:
		return "", fmt.Errorf("unknown provider plugin %q", p.Plugin)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.Plugin == "completions" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		retryAfter := time.Duration(0)
		if seconds, parseErr := strconv.Atoi(resp.Header.Get("Retry-After")); parseErr == nil && seconds > 0 {
			retryAfter = time.Duration(seconds) * time.Second
		}
		return "", &HTTPError{StatusCode: resp.StatusCode, Body: string(responseBody[:min(len(responseBody), 500)]), RetryAfter: retryAfter}
	}

	var result map[string]any
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", err
	}
	if p.Plugin == "completions" {
		choices, _ := result["choices"].([]any)
		if len(choices) == 0 {
			return "", fmt.Errorf("no choices in response")
		}
		choice, _ := choices[0].(map[string]any)
		finishReason, _ := choice["finish_reason"].(string)
		if finishReason == "length" || finishReason == "max_tokens" {
			return "", fmt.Errorf("response truncated: finish_reason=%s", finishReason)
		}
		message, _ := choice["message"].(map[string]any)
		text, _ := message["content"].(string)
		if text == "" {
			return "", fmt.Errorf("empty completion response")
		}
		return text, nil
	}

	stopReason, _ := result["stop_reason"].(string)
	if stopReason == "max_tokens" {
		return "", fmt.Errorf("response truncated: stop_reason=max_tokens")
	}
	content, _ := result["content"].([]any)
	for _, rawBlock := range content {
		block, _ := rawBlock.(map[string]any)
		if block["type"] == "text" {
			text, _ := block["text"].(string)
			if text != "" {
				return text, nil
			}
		}
	}
	return "", fmt.Errorf("no text content in response")
}
