// lumen-server starts the Lumen HTTP API server.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/optakt/lumen/server"
)

func main() {
	cfg := server.DefaultConfig()

	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "BoltDB database path")
	flag.IntVar(&cfg.ContextMaxBeliefs, "max-beliefs", cfg.ContextMaxBeliefs, "max beliefs in /context")
	flag.Float64Var(&cfg.ContextMinConfidence, "min-confidence", cfg.ContextMinConfidence, "min confidence for /context")
	flag.Float64Var(&cfg.IngestMinConfidence, "ingest-confidence", cfg.IngestMinConfidence, "min confidence for /ingest")
	flag.StringVar(&cfg.DefaultFrame, "frame", cfg.DefaultFrame, "default frame for ingested beliefs")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	srv, err := server.New(cfg, logger)
	if err != nil {
		logger.Error("failed to create server", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.ListenAndServe(ctx); err != nil {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
}
