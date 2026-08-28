package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	lumen "github.com/optakt/lumen"
)

func auditPost(srv *Server, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.ServeHTTP(response, req)
	return response
}

func TestNewRejectsCorruptDatabase(t *testing.T) {
	path := t.TempDir() + "/corrupt.db"
	db, err := lumen.OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("frames")).Put([]byte("_all"), []byte("not-json"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.DBPath = path
	if srv, err := New(cfg, slog.Default()); err == nil {
		if srv != nil {
			srv.db.Close()
		}
		t.Fatal("New silently replaced a corrupt persisted store with an empty one")
	}
}

func TestEnsureFrameDoesNotOverwriteExistingPolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBPath = t.TempDir() + "/store.db"
	srv, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.db.Close()

	srv.store.RegisterFrame(lumen.Frame{Name: "permanent", Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}})
	srv.ensureFrame("permanent")

	now := time.Now()
	if err := srv.store.Believe(&lumen.Belief{
		ID: "b", Frame: "permanent", Content: "stable", Confidence: 0.8,
		AssertedAt: now.Add(-30 * 24 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	result, err := srv.store.Query("b", now)
	if err != nil {
		t.Fatal(err)
	}
	if result.CurrentConfidence != 0.8 {
		t.Fatalf("ensureFrame overwrote existing no-decay policy: confidence=%g", result.CurrentConfidence)
	}
}

func TestSelfClaimCanRetryAfterMissingSource(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBPath = t.TempDir() + "/store.db"
	srv, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.db.Close()

	claim := `{"id":"commitment","kind":"derived","content":"A derived commitment","confidence":0.8,"sources":["source"]}`
	if response := auditPost(srv, "/v1/self/claim", claim); response.Code != http.StatusConflict {
		t.Fatalf("first claim: got %d, want conflict", response.Code)
	}
	if response := auditPost(srv, "/v1/records", `{"id":"source","content":"support"}`); response.Code != http.StatusCreated {
		t.Fatalf("assert source: got %d: %s", response.Code, response.Body.String())
	}
	if response := auditPost(srv, "/v1/self/claim", claim); response.Code != http.StatusCreated {
		t.Fatalf("retry claim: got %d: %s", response.Code, response.Body.String())
	}
}
