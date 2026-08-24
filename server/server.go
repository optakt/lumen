// Package server exposes Lumen's belief store over HTTP for agent framework integrations.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	lumen "github.com/optakt/lumen"
	bolt "go.etcd.io/bbolt"
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
		Addr:                 ":3737",
		DBPath:               "lumen.db",
		ContextMaxBeliefs:    10,
		ContextMinConfidence: 0.5,
		IngestMinConfidence:  0.55,
		DefaultFrame:         "reasoning",
	}
}

// Server wraps the belief store and HTTP mux.
type Server struct {
	cfg    Config
	store  *lumen.Store
	db     *bolt.DB
	mux    *http.ServeMux
	logger *slog.Logger
}

// New creates a Server, opening or creating the belief store at cfg.DBPath.
func New(cfg Config, logger *slog.Logger) (*Server, error) {
	db, err := lumen.OpenDB(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	store, err := lumen.LoadStore(db, time.Now())
	if err != nil {
		// Fresh store if the DB is empty.
		store = lumen.NewStore()
	}

	// Ensure the default frame exists.
	if cfg.DefaultFrame != "" {
		store.RegisterFrame(lumen.Frame{
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
	s.mux.HandleFunc("GET /health",       s.handleHealth)
	s.mux.HandleFunc("GET /context",      s.handleContext)
	s.mux.HandleFunc("GET /beliefs",      s.handleListBeliefs)
	s.mux.HandleFunc("POST /records",     s.handleAssertRecord)
	s.mux.HandleFunc("POST /believe",     s.handleBelieve)
	s.mux.HandleFunc("POST /retract",     s.handleRetract)
	s.mux.HandleFunc("POST /ingest",      s.handleIngest)
	s.mux.HandleFunc("GET /explain/{id}", s.handleExplain)
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

	srv := &http.Server{Handler: s}
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

func (s *Server) save() {
	if err := lumen.SaveStore(s.store, s.db); err != nil {
		s.logger.Error("save store", "err", err)
	}
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
