// archaeology performs pairwise semantic correlation analysis on beliefs in a .lm file.
// It uses Voyage AI embeddings to detect which beliefs share underlying evidence
// or conceptual structure — correlations that Bayesian composition should account for.
//
// Usage:
//   archaeology [--api-key KEY] [--threshold 0.45] [--top 20] <file.lm>
//
// Set VOYAGEAI_API_KEY env var or pass --api-key.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	lumen "github.com/optakt/lumen"
	"github.com/optakt/lumen/correlate"
)

func main() {
	apiKey    := flag.String("api-key",   os.Getenv("VOYAGEAI_API_KEY"), "Voyage AI API key")
	threshold := flag.Float64("threshold", 0.45, "correlation threshold for flagging pairs")
	top       := flag.Int("top", 20, "number of top correlated pairs to display")
	flag.Parse()

	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: VOYAGEAI_API_KEY not set (use env var or --api-key)")
		os.Exit(1)
	}
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: archaeology [flags] <file.lm>")
		os.Exit(1)
	}

	// Load the store from the .lm file.
	path := flag.Arg(0)
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		os.Exit(1)
	}
	store := lumen.NewStore()
	now := time.Now()
	if err := lumen.LoadFile(string(src), store, now); err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}

	// Extract beliefs as evidence items.
	beliefs := store.AllBeliefs(now)
	if len(beliefs) == 0 {
		fmt.Println("No beliefs found in store.")
		return
	}

	evidence := make([]correlate.StoreEvidence, 0, len(beliefs))
	for _, b := range beliefs {
		if b.BeliefID == "" || b.Content == "" { continue }
		evidence = append(evidence, correlate.StoreEvidence{
			ID:      b.BeliefID,
			Content: b.Content,
		})
	}

	fmt.Printf("Archaeology: %s — %d beliefs\n\n", path, len(evidence))

	// Run pairwise correlation via Voyage embeddings.
	matrix, err := correlate.AnalyzePairwise(*apiKey, evidence)
	if err != nil {
		fmt.Fprintf(os.Stderr, "correlation error: %v\n", err)
		os.Exit(1)
	}

	// Print summary stats.
	fmt.Printf("Average inter-belief correlation: r=%.3f\n", matrix.AverageCorrelation())
	isolated := matrix.MostIsolated()
	fmt.Printf("Most epistemically isolated: %s\n\n", isolated.ID)

	// Print top pairs above threshold.
	high := matrix.HighCorrelationPairs(*threshold)
	n := len(high)
	if n > *top { n = *top }
	if n == 0 {
		fmt.Printf("No pairs above threshold %.2f — beliefs appear epistemically independent.\n", *threshold)
		return
	}
	fmt.Printf("High-correlation pairs above r=%.2f (top %d of %d):\n\n", *threshold, n, len(high))
	for _, r := range high[:n] {
		fmt.Printf("  r=%.3f  %-30s  ←→  %s\n", r.EstimatedCorrelation, r.IDa, r.IDb)
		fmt.Printf("         %s\n", r.Interpretation)
	}
	fmt.Println()
	fmt.Println("Implication: high-correlation pairs share underlying evidence.")
	fmt.Println("Use BayesianComposeCorrelated to avoid overcounting their contribution.")
}
