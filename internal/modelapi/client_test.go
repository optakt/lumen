package modelapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompleteOpenAICompatible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization not set")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`)
	}))
	defer server.Close()

	client := NewClient()
	text, err := client.Complete(context.Background(), Provider{Name: "test", Model: "m", URL: server.URL, Plugin: "completions"}, "secret", "system", []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if text != `{"ok":true}` {
		t.Fatalf("text = %q", text)
	}
}

func TestCompleteAnthropicSkipsThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"answer"}]}`)
	}))
	defer server.Close()

	client := NewClient()
	text, err := client.Complete(context.Background(), Provider{Name: "test", Model: "m", URL: server.URL, Plugin: "messages"}, "secret", "system", []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if text != "answer" {
		t.Fatalf("text = %q", text)
	}
}

func TestResponseFingerprintIgnoresSchedulingButTracksOutputConfig(t *testing.T) {
	base := Provider{Name: "m", Model: "id", URL: "https://example.test", Plugin: "completions", MaxTokens: 1000, Concurrency: 1, TimeoutSeconds: 30}
	scheduled := base
	scheduled.Concurrency = 8
	scheduled.TimeoutSeconds = 600
	if base.ResponseFingerprint() != scheduled.ResponseFingerprint() {
		t.Fatal("scheduling fields changed response fingerprint")
	}
	changed := base
	changed.MaxTokens = 2000
	if base.ResponseFingerprint() == changed.ResponseFingerprint() {
		t.Fatal("response-affecting field did not change fingerprint")
	}
}

func TestHTTPErrorPreservesRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"rate limited"}`)
	}))
	defer server.Close()
	client := NewClient()
	_, err := client.Complete(context.Background(), Provider{Name: "test", Model: "m", URL: server.URL, Plugin: "completions"}, "secret", "", []Message{{Role: "user", Content: "hi"}})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %T %v", err, err)
	}
	if httpErr.StatusCode != 429 || httpErr.RetryAfter != 7*time.Second {
		t.Fatalf("HTTP error = %#v", httpErr)
	}
}
