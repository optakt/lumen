package server_test

import (
	"bytes"
	"strings"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/optakt/lumen/server"
)

func newTestServer(t *testing.T) *server.Server {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "lumen-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	cfg := server.DefaultConfig()
	cfg.DBPath = f.Name()
	cfg.IngestMinConfidence = 0.3
	cfg.ContextMinConfidence = 0.3
	srv, err := server.New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func post(t *testing.T, srv *server.Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	return w
}

func get(t *testing.T, srv *server.Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	return w
}

func TestHealth(t *testing.T) {
	w := get(t, newTestServer(t), "/v1/health")
	if w.Code != 200 {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestBelieveAndList(t *testing.T) {
	srv := newTestServer(t)
	w := post(t, srv, "/v1/believe", `{"content":"The hard problem of consciousness is real","confidence":0.8}`)
	if w.Code != 201 {
		t.Fatalf("believe: want 201, got %d: %s", w.Code, w.Body)
	}
	var created map[string]string
	_ = json.NewDecoder(w.Body).Decode(&created)
	if created["id"] == "" {
		t.Fatal("no id returned")
	}

	w2 := get(t, srv, "/v1/beliefs")
	if w2.Code != 200 {
		t.Fatalf("list: want 200, got %d", w2.Code)
	}
	var beliefs []map[string]any
	_ = json.NewDecoder(w2.Body).Decode(&beliefs)
	if len(beliefs) == 0 {
		t.Fatal("expected at least one belief")
	}
}

func TestContextText(t *testing.T) {
	srv := newTestServer(t)
	post(t, srv, "/v1/believe", `{"content":"Consciousness involves subjective experience","confidence":0.75}`)

	w := get(t, srv, "/v1/context")
	if w.Code != 200 {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	body := w.Body.String()
	if body == "No active beliefs in store." {
		t.Error("expected beliefs in context output")
	}
	t.Logf("context:\n%s", body)
}

func TestContextJSON(t *testing.T) {
	srv := newTestServer(t)
	post(t, srv, "/v1/believe", `{"content":"GWT has strong empirical support","confidence":0.7}`)

	w := get(t, srv, "/v1/context?format=json")
	if w.Code != 200 {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var out []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected beliefs")
	}
}

func TestIngest(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]string{
		"text": "The study found that meditation reduces anxiety. Therefore, mindfulness therapy appears to have genuine clinical value.",
	})
	w := post(t, srv, "/v1/ingest", string(body))
	if w.Code != 200 {
		t.Fatalf("ingest: want 200, got %d: %s", w.Code, w.Body)
	}
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	t.Logf("ingest: %v", resp)
}

func TestAssertRecord(t *testing.T) {
	srv := newTestServer(t)
	w := post(t, srv, "/v1/records", `{"content":"Babcock et al. 2024 found UV superradiance in tryptophan networks"}`)
	if w.Code != 201 {
		t.Fatalf("want 201, got %d: %s", w.Code, w.Body)
	}
	var created map[string]string
	_ = json.NewDecoder(w.Body).Decode(&created)
	if created["id"] == "" {
		t.Fatal("no id returned")
	}
}

func TestRetract(t *testing.T) {
	srv := newTestServer(t)
	w := post(t, srv, "/v1/records", `{"content":"Initial observation"}`)
	var created map[string]string
	_ = json.NewDecoder(w.Body).Decode(&created)

	body, _ := json.Marshal(map[string]string{"record_id": created["id"], "reason": "superseded"})
	w2 := post(t, srv, "/v1/retract", string(body))
	if w2.Code != 204 {
		t.Fatalf("retract: want 204, got %d: %s", w2.Code, w2.Body)
	}
}

// ─── Self-model tests ─────────────────────────────────────────────────────────

func TestSelfClaim(t *testing.T) {
	srv := newTestServer(t)

	w := post(t, srv, "/v1/self/claim", `{
		"kind": "asserted",
		"content": "The retrodiction problem arises when decay policies are applied retroactively",
		"confidence": 0.85
	}`)
	if w.Code != 201 {
		t.Fatalf("self/claim: want 201, got %d: %s", w.Code, w.Body)
	}
	var created map[string]any
	_ = json.NewDecoder(w.Body).Decode(&created)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("no id returned")
	}
	t.Logf("created claim: %s (frame: %s)", id, created["frame"])

	// It should appear in self/claims
	w2 := get(t, srv, "/v1/self/claims")
	var claims []map[string]any
	_ = json.NewDecoder(w2.Body).Decode(&claims)
	if len(claims) == 0 {
		t.Fatal("expected self claim in list")
	}
}

func TestSelfContext(t *testing.T) {
	srv := newTestServer(t)
	post(t, srv, "/v1/self/claim", `{"kind":"derived","content":"Frame-dependent decay is the correct model for cross-frame beliefs","confidence":0.78}`)
	post(t, srv, "/v1/self/claim", `{"kind":"retrieved","content":"The hard problem of consciousness resists functional reduction","confidence":0.82}`)

	w := get(t, srv, "/v1/self/context")
	if w.Code != 200 {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	body := w.Body.String()
	if !strings.Contains(body, "epistemic commitments") {
		t.Errorf("expected header in context output, got:\n%s", body)
	}
	t.Logf("self/context:\n%s", body)
}

func TestSelfCorrect(t *testing.T) {
	srv := newTestServer(t)

	// Assert an initial claim.
	w := post(t, srv, "/v1/self/claim", `{"kind":"asserted","content":"Illusionism adequately addresses the hard problem","confidence":0.6}`)
	var c map[string]any
	_ = json.NewDecoder(w.Body).Decode(&c)
	priorID := c["id"].(string)

	// Correct it.
	body, _ := json.Marshal(map[string]string{
		"replaces_id": priorID,
		"content":     "Illusionism fails to account for the phenomenal character of experience",
		"reason":      "counterarguments from knowledge argument",
	})
	w2 := post(t, srv, "/v1/self/correct", string(body))
	if w2.Code != 201 {
		t.Fatalf("self/correct: want 201, got %d: %s", w2.Code, w2.Body)
	}
	var correction map[string]any
	_ = json.NewDecoder(w2.Body).Decode(&correction)
	if correction["retracted_id"] != priorID {
		t.Errorf("expected retracted_id=%s, got %v", priorID, correction["retracted_id"])
	}
	t.Logf("correction: %v", correction)
}

// TestLegacyRedirects verifies legacy paths issue 308 (method- and
// body-preserving) redirects with concrete paths and query strings intact.
func TestLegacyRedirects(t *testing.T) {
	srv := newTestServer(t)

	// GET with query string
	req := httptest.NewRequest("GET", "/context?max=5&min_confidence=0.7", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusPermanentRedirect {
		t.Fatalf("GET /context: want 308, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/v1/context?max=5&min_confidence=0.7" {
		t.Errorf("query string lost: Location=%q", loc)
	}

	// POST must be 308 (301 would drop method and body)
	req = httptest.NewRequest("POST", "/ingest", strings.NewReader(`{"text":"x"}`))
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusPermanentRedirect {
		t.Fatalf("POST /ingest: want 308, got %d", w.Code)
	}

	// Parameterised path must substitute the concrete ID
	req = httptest.NewRequest("GET", "/explain/bel-12345", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if loc := w.Header().Get("Location"); loc != "/v1/explain/bel-12345" {
		t.Errorf("path value not preserved: Location=%q", loc)
	}
}

func TestSelfClaimExplicitID(t *testing.T) {
	srv := newTestServer(t)

	// Assert a claim with an explicit ID that lacks the self: prefix.
	w := httptest.NewRecorder()
	body := `{"id":"my-claim","kind":"asserted","content":"Explicit ID test claim","confidence":0.80,"frame":"reasoning"}`
	req := httptest.NewRequest("POST", "/v1/self/claim", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	id, _ := result["id"].(string)
	if id != "self:my-claim" {
		t.Errorf("expected id=self:my-claim, got %q", id)
	}

	// The claim should appear in /v1/self/context.
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/v1/self/context", nil)
	srv.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "Explicit ID test claim") {
		t.Errorf("claim not visible in context: %s", w.Body.String())
	}

	// Biography should resolve without the self: prefix too.
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/v1/self/biography/my-claim", nil)
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("biography: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Correction should also accept the short ID.
	w = httptest.NewRecorder()
	corrBody := `{"replaces_id":"my-claim","content":"Corrected claim","confidence":0.85,"reason":"test correction"}`
	req = httptest.NewRequest("POST", "/v1/self/correct", strings.NewReader(corrBody))
	req.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("correction: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	t.Logf("correction result: %s", w.Body.String())
}
