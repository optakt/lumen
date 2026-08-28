// Package server exposes Lumen's belief store over HTTP for agent framework integrations.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	lumen "github.com/optakt/lumen"
)

// Config holds server configuration.
type Config struct {
	Addr                 string
	DBPath               string
	ContextMaxBeliefs    int
	ContextMinConfidence float64
	IngestMinConfidence  float64
	DefaultFrame         string
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		Addr:                 "127.0.0.1:3737",
		DBPath:               "lumen.db",
		ContextMaxBeliefs:    10,
		ContextMinConfidence: 0.5,
		IngestMinConfidence:  0.55,
		DefaultFrame:         "reasoning",
	}
}

// Server wraps the belief store and HTTP mux.
type Server struct {
	cfg     Config
	store   *lumen.Store
	db      *bolt.DB
	mux     *http.ServeMux
	logger  *slog.Logger
	writeMu sync.Mutex
}

// New creates a Server, opening or creating the belief store at cfg.DBPath.
func New(cfg Config, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	db, err := lumen.OpenDB(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	store, err := lumen.LoadStore(db, time.Now())
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("load store: %w", err)
	}

	// Ensure the default frame exists.
	if cfg.DefaultFrame != "" {
		store.RegisterFrameIfAbsent(lumen.Frame{
			Name:        cfg.DefaultFrame,
			Composition: lumen.CompositionBayesian,
			Decay: lumen.DecayPolicy{
				Kind:     lumen.DecayExponential,
				Halflife: 7 * 24 * time.Hour,
			},
		})
	}

	s := &Server{cfg: cfg, store: store, db: db, mux: http.NewServeMux(), logger: logger}
	s.routes()
	s.routeSelf()
	return s, nil
}

func (s *Server) routes() {
	// Versioned routes (stable API).
	s.mux.HandleFunc("GET /v1/health", s.handleHealth)
	s.mux.HandleFunc("GET /v1/context", s.handleContext)
	s.mux.HandleFunc("GET /v1/beliefs", s.handleListBeliefs)
	s.mux.HandleFunc("POST /v1/records", s.handleAssertRecord)
	s.mux.HandleFunc("POST /v1/believe", s.handleBelieve)
	s.mux.HandleFunc("POST /v1/retract", s.handleRetract)
	s.mux.HandleFunc("POST /v1/ingest", s.handleIngest)
	s.mux.HandleFunc("GET /v1/explain/{id}", s.handleExplain)

	// Legacy redirects — keep existing integrations working.
	//
	// 308 Permanent Redirect, not 301: clients follow 301 on POST by
	// re-issuing GET without the body, silently breaking POST endpoints.
	// 308 preserves both method and body.
	//
	// The target is built from the request itself — path plus query — so
	// parameterised paths (/explain/{id}) and query strings survive the hop.
	for _, from := range []string{
		"GET /health",
		"GET /context",
		"GET /beliefs",
		"POST /records",
		"POST /believe",
		"POST /retract",
		"POST /ingest",
		"GET /explain/{id}",
	} {
		s.mux.HandleFunc(from, redirectToV1)
	}
}

// redirectToV1 issues a 308 to the /v1-prefixed equivalent of the request,
// preserving the concrete path segments and the query string.
func redirectToV1(w http.ResponseWriter, r *http.Request) {
	target := "/v1" + r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server and blocks until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.Addr, err)
	}
	s.logger.Info("lumen server listening", "addr", ln.Addr())

	srv := &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		return s.db.Close()
	case err := <-errCh:
		_ = s.db.Close()
		return err
	}
}

func (s *Server) save() error {
	if err := lumen.SaveStore(s.store, s.db); err != nil {
		s.logger.Error("save store", "err", err)
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errResponse struct {
	Error string `json:"error"`
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errResponse{Error: msg})
}
