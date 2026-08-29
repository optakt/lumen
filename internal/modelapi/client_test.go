package modelapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
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
