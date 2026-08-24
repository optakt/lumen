// Command timeline renders a Lumen belief store's assertion history as a
// readable chronological narrative — when each record and belief entered
// the store, what it depended on, and what it enabled.
//
// Usage: timeline <file.lm> [--from 2026-01-01] [--to 2026-12-31] [--dot]
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	lumen "github.com/optakt/lumen"
)

func main() {
	fromStr := flag.String("from", "", "start date filter (YYYY-MM-DD)")
	toStr   := flag.String("to", "", "end date filter (YYYY-MM-DD)")
	dot     := flag.Bool("dot", false, "output Graphviz DOT format")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: timeline [--from DATE] [--to DATE] [--dot] <file.lm>")
		os.Exit(1)
	}

	now := time.Now()
	store := lumen.NewStore()
	for _, f := range []lumen.Frame{
		{Name: "philosophical", Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}},
		{Name: "empirical",     Decay: lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 5 * 365 * 24 * time.Hour}},
		{Name: "contemporary",  Decay: lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 10 * 365 * 24 * time.Hour}},
		{Name: "reasoning",     Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}},
		{Name: "test",          Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}},
	} {
		store.RegisterFrame(f)
	}
	if err := lumen.LoadFileWithImports(flag.Arg(0), store, now); err != nil {
		fmt.Fprintf(os.Stderr, "load error: %v\n", err)
		os.Exit(1)
	}

	var from, to time.Time
	if *fromStr != "" {
		t, err := time.Parse("2006-01-02", *fromStr)
		if err != nil { fmt.Fprintf(os.Stderr, "invalid --from: %v\n", err); os.Exit(1) }
		from = t
	}
	if *toStr != "" {
		t, err := time.Parse("2006-01-02", *toStr)
		if err != nil { fmt.Fprintf(os.Stderr, "invalid --to: %v\n", err); os.Exit(1) }
		to = t
	}

	events := store.Temporal.Timeline()

	// Filter
	var filtered []lumen.TemporalEvent
	for _, ev := range events {
		if !from.IsZero() && ev.AssertedAt.Before(from) { continue }
		if !to.IsZero()   && ev.AssertedAt.After(to)    { continue }
		filtered = append(filtered, ev)
	}

	if *dot {
		renderDOT(filtered, store, now)
	} else {
		renderText(filtered, store, now)
	}
}

func renderText(events []lumen.TemporalEvent, store *lumen.Store, now time.Time) {
	if len(events) == 0 {
		fmt.Println("No events in the specified range.")
		return
	}

	fmt.Printf("=== Temporal Timeline (%d events) ===\n\n", len(events))

	// Group by day
	type day struct {
		date   string
		events []lumen.TemporalEvent
	}
	var days []day
	for _, ev := range events {
		d := ev.AssertedAt.Format("2006-01-02")
		if len(days) == 0 || days[len(days)-1].date != d {
			days = append(days, day{date: d})
		}
		days[len(days)-1].events = append(days[len(days)-1].events, ev)
	}

	for _, d := range days {
		fmt.Printf("── %s ──\n", d.date)
		for _, ev := range d.events {
			prefix := "○"
			if ev.Kind == "belief" {
				prefix = "●"
			}
			// Get content for this node
			content := store.ContentFor(ev.NodeID)
			if len(content) > 70 {
				content = content[:67] + "..."
			}
			fmt.Printf("  %s %s  %s  [%s]\n",
				prefix,
				ev.AssertedAt.Format("15:04"),
				content,
				ev.NodeID,
			)
			if len(ev.EnabledBy) > 0 {
				fmt.Printf("    ↳ depends on: %s\n", strings.Join(ev.EnabledBy, ", "))
			}
			// What does this enable?
			downstream := store.Temporal.CounterfactualRemoval(ev.NodeID)
			if len(downstream) > 0 && len(downstream) <= 4 {
				fmt.Printf("    ↗ enables: %s\n", strings.Join(downstream, ", "))
			} else if len(downstream) > 4 {
				fmt.Printf("    ↗ enables: %d downstream nodes\n", len(downstream))
			}
		}
		fmt.Println()
	}

	// Summary
	records, beliefs := 0, 0
	for _, ev := range events {
		if ev.Kind == "record" { records++ } else { beliefs++ }
	}
	fmt.Printf("Total: %d records (○), %d beliefs (●)\n", records, beliefs)
}

func renderDOT(events []lumen.TemporalEvent, store *lumen.Store, now time.Time) {
	fmt.Println("digraph timeline {")
	fmt.Println("  rankdir=LR;")
	fmt.Println("  node [fontname=Helvetica fontsize=10];")
	fmt.Println()

	// Nodes
	for _, ev := range events {
		content := store.ContentFor(ev.NodeID)
		if len(content) > 40 {
			content = content[:37] + "..."
		}
		shape := "box"
		if ev.Kind == "belief" {
			shape = "ellipse"
		}
		label := fmt.Sprintf("%s\\n%s\\n%s", ev.NodeID, ev.AssertedAt.Format("2006-01-02"), content)
		fmt.Printf("  %q [shape=%s label=%q];\n", ev.NodeID, shape, label)
	}

	fmt.Println()

	// Edges (enablement)
	for _, ev := range events {
		for _, src := range ev.EnabledBy {
			fmt.Printf("  %q -> %q [label=\"enables\"];\n", src, ev.NodeID)
		}
	}

	fmt.Println("}")
}
