package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	lumen "github.com/optakt/lumen"
)

type finding struct {
	id          string
	content     string
	frame       string
	declared    float64
	discrepancy float64
	direction   string
}

func main() {
	threshold := flag.Float64("threshold", 0.15, "minimum confidence discrepancy to flag")
	output := flag.String("output", "", "write suggested .lm patch to this file")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: calibrate [--threshold N] [--output file.lm] <file.lm>")
		os.Exit(1)
	}
	now := time.Now()
	store := lumen.NewStore()
	for _, f := range []lumen.Frame{
		{Name: "philosophical", Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}},
		{Name: "empirical", Decay: lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 5 * 365 * 24 * time.Hour}},
		{Name: "contemporary", Decay: lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 10 * 365 * 24 * time.Hour}},
		{Name: "reasoning", Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}},
		{Name: "test", Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}},
	} {
		store.RegisterFrame(f)
	}
	if err := lumen.LoadFileWithImports(flag.Arg(0), store, now); err != nil {
		fmt.Fprintf(os.Stderr, "load error: %v\n", err)
		os.Exit(1)
	}

	beliefs := store.AllBeliefs(now)
	var findings []finding

	for _, b := range beliefs {
		conf := b.CurrentConfidence
		// Flag beliefs where decay has caused significant drift (below 50%, non-suspect)
		if b.State != lumen.BeliefSuspect && conf < 0.5 && conf > 0.0 {
			findings = append(findings, finding{
				id:          b.BeliefID,
				content:     trunc(b.Content, 60),
				frame:       b.Frame,
				declared:    conf,
				discrepancy: 0.5 - conf,
				direction:   "stale",
			})
		}
		// Flag beliefs anchored at suspiciously round numbers (possible lazy calibration)
		for _, v := range []float64{0.5, 0.6, 0.7, 0.8, 0.9} {
			if math.Abs(conf-v) < 0.005 && conf > 0 {
				// Only flag if we also have derivation evidence to compare against
				_ = v // placeholder — full calibration needs explicit priors
				break
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		return findings[i].discrepancy > findings[j].discrepancy
	})

	fmt.Printf("=== Calibration Report: %s ===\n\n", flag.Arg(0))
	fmt.Printf("Analyzed %d beliefs (threshold: %.0f%%)\n\n", len(beliefs), *threshold*100)

	if len(findings) == 0 {
		fmt.Println("No calibration issues detected.")
		fmt.Println("\nFor posterior-based calibration, add explicit 'prior:' and evidence")
		fmt.Println("declarations to your .lm files to enable full Bayesian comparison.")
	} else {
		fmt.Printf("%d belief(s) flagged:\n\n", len(findings))
		for _, f := range findings {
			fmt.Printf("  [%s] %.0f%% %q\n", f.frame, f.declared*100, f.content)
			fmt.Printf("    Decay has eroded confidence below 50%%. Re-assert with fresh evidence\n")
			fmt.Printf("    or explicitly set confidence: 0.00 to mark expired.\n\n")
		}
	}

	// Store health
	fmt.Println("=== Store Health ===")
	stats := store.Graph.Stats()
	var parts []string
	for k, v := range stats {
		parts = append(parts, fmt.Sprintf("%s=%d", k, v))
	}
	sort.Strings(parts)
	fmt.Printf("Graph: %s\n", strings.Join(parts, " "))
	ec, mc := store.Entities.EntityStats()
	fmt.Printf("Entities: %d registered, %d mentions\n", ec, mc)
	suspect := 0
	for _, b := range beliefs {
		if b.State == lumen.BeliefSuspect {
			suspect++
		}
	}
	if suspect > 0 {
		fmt.Printf("⚠  %d suspect belief(s)\n", suspect)
	} else {
		fmt.Println("✓  No suspect beliefs")
	}

	// Optional patch output
	if *output != "" && len(findings) > 0 {
		var p strings.Builder
		fmt.Fprintf(&p, "# Calibration patch — review before applying\n\n")
		for _, f := range findings {
			fmt.Fprintf(&p, "# stale: %q\n# believe %s in %s\n#   confidence: 0.00\n\n", f.content, f.id, f.frame)
		}
		if err := os.WriteFile(*output, []byte(p.String()), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		} else {
			fmt.Printf("\nPatch written to %s\n", *output)
		}
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
