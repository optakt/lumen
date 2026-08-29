// Package modelapi provides the small provider-neutral chat client used by
// Lumen's black-box experiments. It deliberately exposes only messages and
// text responses: provider metadata must not enter the measured trajectory.
package modelapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Provider describes one OpenAI- or Anthropic-compatible endpoint.
type Provider struct {
	Name      string `json:"name"`
	Model     string `json:"model"`
	URL       string `json:"url"`
	APIKey    string `json:"api_key_env"`
	Plugin    string `json:"plugin"` // "messages" or "completions"
	MaxTokens int    `json:"max_tokens,omitempty"`
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
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 2 * time.Minute}
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
		if !strings.HasSuffix(base, "/v1") && !strings.Contains(base, "/v1/") && !strings.Contains(base, "/v1beta/") {
			base += "/v1"
		}
		endpoint = base + "/chat/completions"
		providerMessages := make([]Message, 0, len(messages)+1)
		if system != "" {
			providerMessages = append(providerMessages, Message{Role: "system", Content: system})
		}
		providerMessages = append(providerMessages, messages...)
		payload = map[string]any{
			"model":                 p.Model,
			"messages":              providerMessages,
			"max_completion_tokens": maxTokens,
		}
	case "messages":
		endpoint = strings.TrimSuffix(p.URL, "/") + "/v1/messages"
		payload = map[string]any{
			"model":      p.Model,
			"system":     system,
			"messages":   messages,
			"max_tokens": maxTokens,
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

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(responseBody[:min(len(responseBody), 500)]))
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
		message, _ := choice["message"].(map[string]any)
		text, _ := message["content"].(string)
		if text == "" {
			return "", fmt.Errorf("empty completion response")
		}
		return text, nil
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
