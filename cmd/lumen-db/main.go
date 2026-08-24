// Command lumen-db is a persistent Lumen belief store session.
// It opens (or creates) a BoltDB database, loads the stored state,
// and provides an interactive REPL for querying and updating beliefs.
//
// Usage: lumen-db [--db path/to/store.db] [--load file.lm]
//
// Commands:
//   load <file.lm>      — load a .lm file into the store (and save)
//   analyze <file.txt>  — extract beliefs from free text and load them
//   query <id>          — query a belief's current state
//   explain <id>        — narrative explanation of a belief
//   conflict            — scan for epistemic conflicts
//   timeline            — render assertion history
//   calibrate           — run calibration check
//   save                — explicitly save to disk
//   exit                — exit (also saves)
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	lumen "github.com/optakt/lumen"
)

// shellSplit splits a command line respecting double-quoted strings.
// "hello world" becomes a single token without the quotes.
func shellSplit(s string) []string {
	var tokens []string
	var cur []rune
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			if len(cur) > 0 {
				tokens = append(tokens, string(cur))
				cur = cur[:0]
			}
		default:
			cur = append(cur, r)
		}
	}
	if len(cur) > 0 {
		tokens = append(tokens, string(cur))
	}
	return tokens
}

var defaultFrames = []lumen.Frame{
	{Name: "philosophical", Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}},
	{Name: "empirical",     Decay: lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 5 * 365 * 24 * time.Hour}},
	{Name: "contemporary",  Decay: lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 10 * 365 * 24 * time.Hour}},
	{Name: "reasoning",     Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}},
}

func main() {
	dbPath  := flag.String("db",   "lumen.db", "path to BoltDB store")
	loadLM  := flag.String("load", "",         "load a .lm file on startup")
	flag.Parse()

	now := time.Now()

	// Open database
	db, err := lumen.OpenDB(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open store: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Load or initialize store
	store, err := lumen.LoadStore(db, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot load store: %v\n", err)
		os.Exit(1)
	}
	// Ensure default frames exist
	for _, f := range defaultFrames {
		store.RegisterFrame(f)
	}

	// Count what we loaded
	beliefs := store.AllBeliefs(now)
	fmt.Printf("lumen-db — %s\n", *dbPath)
	fmt.Printf("%d beliefs, %d records in store\n", len(beliefs), store.RecordCount())
	fmt.Println("Type 'help' for commands.")

	// Load file if specified
	if *loadLM != "" {
		if err := loadFile(store, *loadLM, now); err != nil {
			fmt.Fprintf(os.Stderr, "load error: %v\n", err)
		} else {
			lumen.SaveStore(store, db)
		}
	}

	// REPL
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		now = time.Now()
		parts := strings.Fields(line)
		cmd   := parts[0]
		args  := parts[1:]

		switch cmd {
		case "help":
			printHelp()
		case "exit", "quit":
			lumen.SaveStore(store, db)
			fmt.Println("Saved. Goodbye.")
			return
		case "export":
			format := "markdown"
			outPath := ""
			if len(args) >= 1 { format = args[0] }
			if len(args) >= 2 { outPath = args[1] }
			var content string
			switch format {
			case "json":
				data, err := store.ExportJSON(now)
				if err != nil { fmt.Printf("export error: %v\n", err); continue }
				content = string(data)
			case "lm":
				content = store.ExportLM(now)
			default:
				content = store.ExportMarkdown("Knowledge Base", now)
			}
			if outPath != "" {
				if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
					fmt.Printf("write error: %v\n", err)
				} else {
					fmt.Printf("Exported to %s\n", outPath)
				}
			} else {
				fmt.Print(content)
			}
		case "save":
			if err := lumen.SaveStore(store, db); err != nil {
				fmt.Printf("save error: %v\n", err)
			} else {
				fmt.Println("Saved.")
			}
		case "dot":
			// dot [file.dot] — export belief graph as Graphviz DOT
			opts := lumen.DefaultDotOptions(now)
			dot := store.ExportDot(opts)
			if len(args) > 0 {
				if err := os.WriteFile(args[0], []byte(dot), 0o644); err != nil {
					fmt.Printf("write error: %v\n", err)
				} else {
					fmt.Printf("wrote %s (%d bytes) — render with: dot -Tsvg %s -o graph.svg\n", args[0], len(dot), args[0])
				}
			} else {
				fmt.Print(dot)
			}

		case "assert":
			// assert <id> <frame> "<content>" [at <YYYY-MM-DD>]
			// assert a new record into the store
			if len(args) < 3 {
				fmt.Println("usage: assert <id> <frame> \"<content>\" [at YYYY-MM-DD]")
				continue
			}
			rid, rframe, rcontent := args[0], args[1], args[2]
			ts := now
			for i, a := range args {
				if a == "at" && i+1 < len(args) {
					if t, err := time.Parse("2006-01-02", args[i+1]); err == nil { ts = t }
				}
			}
			if err := store.Assert(&lumen.Record{ID: rid, Frame: rframe, Content: rcontent, Timestamp: ts}); err != nil {
				fmt.Printf("assert error: %v\n", err)
			} else {
				lumen.SaveStore(store, db)
				fmt.Printf("asserted record %s in frame %s\n", rid, rframe)
			}
		case "believe":
			// believe <id> <frame> <conf> "<content>" [from r1,r2,...]
			// assert a new belief into the store
			if len(args) < 4 {
				fmt.Println("usage: believe <id> <frame> <conf 0-1> \"<content>\" [from r1,r2,...]")
				continue
			}
			bid, bframe, bconfStr, bcontent := args[0], args[1], args[2], args[3]
			var bconf float64
			if n, _ := fmt.Sscanf(bconfStr, "%f", &bconf); n != 1 || bconf < 0 || bconf > 1 {
				fmt.Println("confidence must be a float in [0, 1]")
				continue
			}
			var derivation []string
			for i, a := range args {
				if a == "from" && i+1 < len(args) {
					for _, d := range strings.Split(args[i+1], ",") {
						if d = strings.TrimSpace(d); d != "" { derivation = append(derivation, d) }
					}
				}
			}
			if err := store.Believe(&lumen.Belief{
				ID: bid, Frame: bframe, Content: bcontent,
				Confidence: bconf, AssertedAt: now, Derivation: derivation,
			}); err != nil {
				fmt.Printf("believe error: %v\n", err)
			} else {
				lumen.SaveStore(store, db)
				fmt.Printf("believed %s at %.0f%% in frame %s\n", bid, bconf*100, bframe)
			}
		case "retract":
			// retract <record-id> [reason]
			if len(args) == 0 {
				fmt.Println("usage: retract <record-id> [reason]")
				continue
			}
			reason := ""
			if len(args) > 1 { reason = strings.Join(args[1:], " ") }
			// Preview impact BEFORE retraction (markSuspect changes state).
			impacted, _ := store.ImpactScan(args[0], now)
			if err := store.Retract(args[0], reason, now); err != nil {
				fmt.Printf("retract error: %v\n", err)
			} else {
				lumen.SaveStore(store, db)
				if len(impacted) == 0 {
					fmt.Printf("retracted %s — no downstream beliefs affected\n", args[0])
				} else {
					fmt.Printf("retracted %s — %d beliefs now suspect:\n", args[0], len(impacted))
					for i, e := range impacted {
						if i >= 5 { fmt.Printf("  ... and %d more\n", len(impacted)-5); break }
						fmt.Printf("  %s\n", e.BeliefID)
					}
					fmt.Printf("Run 'impact %s' for full details.\n", args[0])
				}
			}

		case "load":
			if len(args) == 0 { fmt.Println("usage: load <file.lm>"); continue }
			if err := loadFile(store, args[0], now); err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				lumen.SaveStore(store, db)
			}
		case "analyze":
			if len(args) == 0 { fmt.Println("usage: analyze <file.txt>"); continue }
			src, err := os.ReadFile(args[0])
			if err != nil { fmt.Printf("error: %v\n", err); continue }
			analysis := lumen.AnalyzeText(string(src))
			fmt.Printf("Extracted: %d records, %d beliefs, frame=%s\n",
				len(analysis.Records), len(analysis.Beliefs), analysis.Frame)
			// Load into store
			loadAnalysis(store, analysis, now)
			lumen.SaveStore(store, db)
		case "query":
			if len(args) == 0 { fmt.Println("usage: query <id>"); continue }
			q, err := store.Query(args[0], now)
			if err != nil { fmt.Printf("not found: %v\n", err); continue }
			fmt.Printf("[%s] %.0f%% — %s\n", q.Frame, q.CurrentConfidence*100, q.Content)
		case "provenance":
			if len(args) == 0 { fmt.Println("usage: provenance <id>"); continue }
			chain, err := store.ProvenanceChain(args[0], now)
			if err != nil { fmt.Printf("error: %v\n", err); continue }
			fmt.Print(chain.Render())
			weak := chain.WeakestLink()
			if weak != nil {
				fmt.Printf("Weakest link: %s (%.0f%%) — %s\n", weak.ID, weak.Confidence*100, truncate(weak.Content, 50))
			}
			paths := chain.ConfidencePaths()
			fmt.Printf("Confidence paths:\n")
			for _, p := range paths {
				var ids []string
				for _, step := range p.Steps { ids = append(ids, step.ID) }
				fmt.Printf("  %s → min=%.0f%%\n", strings.Join(ids, " → "), p.MinConfidence()*100)
			}
		case "explain":
			if len(args) == 0 { fmt.Println("usage: explain <id>"); continue }
			expl, err := store.Explain(args[0], now)
			if err != nil { fmt.Printf("error: %v\n", err); continue }
			fmt.Println(expl)
		case "validate":
			// validate — check store for consistency issues
			issues := store.Validate()
			if len(issues) == 0 {
				fmt.Println("store is consistent — no issues found")
				continue
			}
			errCount, warnCount := 0, 0
			for _, i := range issues {
				fmt.Println(" ", i)
				if i.Kind == "error" { errCount++ } else { warnCount++ }
			}
			fmt.Printf("\n%d error(s), %d warning(s)\n", errCount, warnCount)

		case "conflict":
			conflicts := store.ConflictScan(now)
			if len(conflicts) == 0 {
				fmt.Println("No conflicts detected.")
				continue
			}
			for _, c := range conflicts {
				fmt.Printf("[%.0f%%] %s: %s\n  %s\n", c.Strength*100, c.Kind, c.BeliefA+" ↔ "+c.BeliefB, c.Explanation)
			}
		case "timeline":
			events := store.Temporal.Timeline()
			fmt.Printf("%d events in timeline\n", len(events))
			for _, ev := range events {
				content := store.ContentFor(ev.NodeID)
				if len(content) > 60 { content = content[:57] + "..." }
				fmt.Printf("  %s  %-8s  %s\n", ev.AssertedAt.Format("2006-01-02"), ev.NodeID, content)
			}
		case "list":
			beliefs := store.AllBeliefs(now)
			sort.Slice(beliefs, func(i, j int) bool {
				return beliefs[i].CurrentConfidence > beliefs[j].CurrentConfidence
			})
			for _, b := range beliefs {
				// Skip contracted beliefs — they are soft-deleted.
				if b.State == lumen.BeliefSuperseded { continue }
				stateMarker := ""
				if b.State == lumen.BeliefSuspect { stateMarker = " ⚠" }
				fmt.Printf("  [%s] %.0f%% %-20s  %s%s\n",
					b.Frame, b.CurrentConfidence*100, b.BeliefID,
					truncate(b.Content, 55), stateMarker)
			}
		case "calibrate":
			// calibrate — flag beliefs that may need review.
			// Flags: low confidence (<50%), suspect state, or stale derivers.
			beliefs := store.AllBeliefs(now)
			type flag struct{ level, id, frame, note string; conf float64 }
			var flags []flag
			for _, b := range beliefs {
				if b.State == lumen.BeliefSuperseded { continue }
				switch {
				case b.State == lumen.BeliefSuspect:
					flags = append(flags, flag{"SUSPECT", b.BeliefID, b.Frame, "one or more sources retracted", b.CurrentConfidence})
				case b.CurrentConfidence < 0.30:
					flags = append(flags, flag{"LOW", b.BeliefID, b.Frame, fmt.Sprintf("%.0f%% — consider revising or retracting", b.CurrentConfidence*100), b.CurrentConfidence})
				case b.CurrentConfidence < 0.50:
					flags = append(flags, flag{"WEAK", b.BeliefID, b.Frame, fmt.Sprintf("%.0f%% — below useful confidence threshold", b.CurrentConfidence*100), b.CurrentConfidence})
				}
			}
			// Also check for stale derivers (on active beliefs only).
			for _, b := range beliefs {
				if b.State != lumen.BeliefActive { continue }
				if stale := store.StaleDerivers(b.BeliefID, now); len(stale) > 0 {
					flags = append(flags, flag{"STALE", b.BeliefID, b.Frame,
						fmt.Sprintf("derives from stale sources: %s", strings.Join(stale, ", ")),
						b.CurrentConfidence})
				}
			}
			if len(flags) == 0 {
				fmt.Println("No calibration issues.")
				continue
			}
			for _, f := range flags {
				content := store.ContentFor(f.id)
				if len(content) > 50 { content = content[:47] + "..." }
				fmt.Printf("  [%-7s]  %-30s  [%s]  %s\n", f.level, f.id, f.frame, f.note)
				if content != "" { fmt.Printf("           %q\n", content) }
			}
		case "search":
			if len(args) == 0 { fmt.Println("usage: search <terms>"); continue }
			idx := store.BuildSearchIndex()
			query := strings.Join(args, " ")
			results := store.Search(idx, query, 10)
			if len(results) == 0 { fmt.Println("No results."); continue }
			for _, r := range results {
				fmt.Printf("  [%.2f] %-8s %-20s  %s\n",
					r.Similarity, r.Kind, r.NodeID, truncate(r.Content, 55))
			}
		case "find":
			if len(args) == 0 { fmt.Println("usage: find <query>  e.g. find confidence > 0.7 AND frame = philosophical"); continue }
			qstr := strings.Join(args, " ")
			matches, err := store.QueryBeliefs(qstr, now)
			if err != nil { fmt.Printf("query error: %v\n", err); continue }
			if len(matches) == 0 { fmt.Println("No matches."); continue }
			for _, m := range matches {
				state := ""
				if m.State == lumen.BeliefSuspect { state = " [SUSPECT]" }
				fmt.Printf("  [%s] %.0f%%%s %-20s  %s\n",
					m.Frame, m.CurrentConfidence*100, state, m.BeliefID,
					truncate(m.Content, 55))
			}
			fmt.Printf("  (%d matches)\n", len(matches))
		case "bio":
			// bio <belief-id> [threshold]
			// Produces a complete epistemic biography: confidence arc,
			// mind changes, source history, retractions, frame crossings.
			if len(args) == 0 {
				fmt.Println("usage: bio <belief-id> [threshold]")
				fmt.Println("       threshold: min delta to qualify as a mind-change (default 0.05)")
				continue
			}
			threshold := 0.05
			if len(args) >= 2 {
				if t, err := strconv.ParseFloat(args[1], 64); err == nil {
					threshold = t
				}
			}
			bio, err := store.EpistemicBiography(args[0], threshold, now)
			if err != nil {
				fmt.Printf("bio error: %v\n", err)
				continue
			}
			fmt.Print(lumen.RenderBiography(bio))
		case "queries":
			// queries — list all named queries loaded from .lm files
			all := store.AllQueries()
			if len(all) == 0 {
				fmt.Println("No named queries registered. Load a .lm file containing query declarations.")
				continue
			}
			for _, q := range all {
				fmt.Printf("  %-20s  target=%-15s  select=%s", q.ID, q.Target, q.Select)
				if q.Since != "" { fmt.Printf("  since=%s", q.Since) }
				if q.Where != "" { fmt.Printf("  where=%s", q.Where) }
				fmt.Println()
			}
		case "run":
			// run <belief-id> <select-kind> [since <date>] [where <predicate...>]
			// Executes an epistemic archaeology query against the store.
			// Select kinds: confidence-changes, source-changes, retraction-events
			if len(args) < 2 {
				fmt.Println("usage: run <belief-id> <select-kind> [since <date>] [where <predicate>]")
				fmt.Println("       select-kind: confidence-changes | source-changes | retraction-events")
				fmt.Println("       example: run hard-problem confidence-changes where change > 0.1")
				continue
			}
			// If only one arg is given, treat it as a named query ID.
			if len(args) == 1 {
				ar, err := store.RunQueryByID(args[0], now)
				if err != nil {
					fmt.Printf("query error: %v\n", err)
				} else {
					fmt.Print(lumen.RenderArchiveResult(ar))
				}
				continue
			}
			q := lumen.ParsedQuery{
				ID:     "adhoc",
				Target: args[0],
				Select: args[1],
			}
			// Parse optional since / where clauses from remaining args.
			rest := args[2:]
			for i := 0; i < len(rest); i++ {
				switch rest[i] {
				case "since":
					if i+1 < len(rest) {
						i++
						q.Since = rest[i]
					}
				case "where":
					if i+1 < len(rest) {
						q.Where = strings.Join(rest[i+1:], " ")
						i = len(rest) // consume rest
					}
				}
			}
			ar, err := store.ExecuteQuery(q, now)
			if err != nil {
				fmt.Printf("query error: %v\n", err)
				continue
			}
			fmt.Print(lumen.RenderArchiveResult(ar))
		case "health":
			// health <belief-id> — epistemic health score
			if len(args) == 0 {
				fmt.Println("usage: health <belief-id>")
				continue
			}
			belief, hErr := store.BeliefHealth(args[0], now)
			if hErr != nil {
				fmt.Printf("health error: %v\n", hErr)
				continue
			}
			fmt.Printf("Grade: %s  Score: %.0f/100\n", belief.Grade, belief.Score)
			for _, c := range belief.Components {
				fmt.Printf("  %-20s %5.1f%% (weight %.0f%%)  %s\n",
					c.Name, c.Value, c.Weight*100, c.Note)
			}
			if len(belief.Warnings) > 0 {
				for _, w := range belief.Warnings {
					fmt.Printf("  ⚠ %s\n", w)
				}
			}

		case "advance":
			// advance <duration> — move the reference clock forward
			// e.g. "advance 30d", "advance 1y", "advance 6m"
			if len(args) == 0 {
				fmt.Printf("current time: %s\n", now.Format("2006-01-02"))
				continue
			}
			if args[0] == "reset" {
				now = time.Now()
				fmt.Printf("time reset to %s\n", now.Format("2006-01-02"))
				continue
			}
			d, advErr := parseDuration(args[0])
			if advErr != nil {
				fmt.Printf("advance: %v (use e.g. 30d, 6m, 1y, or 'reset')\n", advErr)
				continue
			}
			now = now.Add(d)
			fmt.Printf("time advanced to %s\n", now.Format("2006-01-02"))

		case "counterfactual", "cf":
			// counterfactual <belief-id> <source-id> — what-if excluding one source
			if len(args) < 2 {
				fmt.Println("usage: counterfactual <belief-id> <source-id>")
				continue
			}
			full, cf, delta, cfErr := store.CounterfactualConfidence(args[0], args[1], now)
			if cfErr != nil {
				fmt.Printf("counterfactual error: %v\n", cfErr)
				continue
			}
			fullMid := (full.Lo + full.Hi) / 2
			cfMid   := (cf.Lo + cf.Hi) / 2
			fmt.Printf("Belief:   %s\n", args[0])
			fmt.Printf("Excluding: %s\n", args[1])
			fmt.Printf("Full confidence:         [%.1f%%, %.1f%%]  mid=%.1f%%\n", full.Lo*100, full.Hi*100, fullMid*100)
			fmt.Printf("Without %s:  [%.1f%%, %.1f%%]  mid=%.1f%%\n", args[1], cf.Lo*100, cf.Hi*100, cfMid*100)
			if delta < -0.05 {
				fmt.Printf("Impact: %.1f pp drop — this source is load-bearing\n", -delta*100)
			} else if delta < 0 {
				fmt.Printf("Impact: %.1f pp drop — marginal contribution\n", -delta*100)
			} else {
				fmt.Printf("Impact: %.1f pp — negligible\n", math.Abs(delta)*100)
			}

		case "impact":
			// impact <source-id> — blast radius of retracting this record or belief
			if len(args) == 0 {
				fmt.Println("usage: impact <source-id>")
				continue
			}
			entries, iErr := store.ImpactScan(args[0], now)
			if iErr != nil {
				fmt.Printf("error: %v\n", iErr)
				continue
			}
			if len(entries) == 0 {
				fmt.Printf("No active beliefs depend on %s\n", args[0])
				continue
			}
			fmt.Printf("Impact of retracting %s — %d beliefs affected:\n\n", args[0], len(entries))
			for i, e := range entries {
				dropStr := fmt.Sprintf("−%.0f pp", e.Drop*100)
				if e.Drop < 0.001 { dropStr = "stable" }
				linkStr := "transitive"
				if e.DirectlyLinked { linkStr = "direct" }
				fmt.Printf("  %2d.  [%3.0f%% → %3.0f%%]  %s  %s (hop %d)\n",
					i+1, e.CurrentConf*100, e.EstimatedConf*100, dropStr, e.BeliefID, e.Distance)
				fmt.Printf("       %s\n", linkStr)
				content := e.BeliefContent
				if len(content) > 70 { content = content[:67] + "..." }
				fmt.Printf("       %q\n\n", content)
			}

		case "fragility":
			// fragility [n] — store-wide fragility scan, ranked by drop
			n := 10
			if len(args) > 0 { fmt.Sscanf(args[0], "%d", &n) }
			entries := store.FragilityScan(now)
			if len(entries) == 0 {
				fmt.Println("no beliefs with derivation sources to scan")
				continue
			}
			if n > len(entries) { n = len(entries) }
			fmt.Printf("Fragility scan — top %d (most fragile first):\n\n", n)
			for i, e := range entries[:n] {
				dropStr := fmt.Sprintf("−%.0f pp", e.Drop*100)
				if e.Drop < 0.001 { dropStr = "stable" }
				fmt.Printf("  %2d.  [%3.0f%% → %3.0f%%]  %s  %s\n",
					i+1, e.CurrentConf*100, e.ConfWithout*100, dropStr, e.BeliefID)
				fmt.Printf("       weakest: %s (%s)  min-cut: %d  total sources: %d\n",
					e.WeakestSource, e.WeakestKind, e.MinCut, e.TotalSources)
				content := e.BeliefContent
				if len(content) > 70 { content = content[:67] + "..." }
				fmt.Printf("       %q\n\n", content)
			}

		case "sensitivity":
			// sensitivity <belief-id> — show source contributions and weakest link
			if len(args) == 0 {
				fmt.Println("usage: sensitivity <belief-id>")
				continue
			}
			chain, sErr := store.ProvenanceChain(args[0], now)
			if sErr != nil {
				fmt.Printf("sensitivity error: %v\n", sErr)
				continue
			}
			fmt.Printf("Provenance depth: %d  Records: %d\n\n", chain.MaxDepth, chain.TotalRecords)
			// Show sources sorted by confidence (ascending = most fragile first).
			type srcLine struct{ id string; conf float64; kind string; foundational bool; retracted bool }
			var sources []srcLine
			for id, node := range chain.Nodes {
				if id == chain.Root { continue }
				sources = append(sources, srcLine{id, node.Confidence, node.Kind, node.Foundational, node.Retracted})
			}
			sort.Slice(sources, func(i, j int) bool { return sources[i].conf < sources[j].conf })
			for _, s := range sources {
				marker := ""
				if s.foundational { marker = " ⚑ foundational" }
				if s.retracted    { marker = " ✗ RETRACTED" }
				fmt.Printf("  %5.0f%%  %-8s  %-30s%s\n", s.conf*100, s.kind, s.id, marker)
			}
			if wl := chain.WeakestLink(); wl != nil {
				fmt.Printf("\nWeakest link (excluding foundational): %s (%.0f%%)\n", wl.ID, wl.Confidence*100)
			} else {
				fmt.Println("\nNo non-foundational weak links.")
			}

		case "contracted":
			// contracted — list beliefs that were contracted and can be recovered
			contracted := store.ContractedBeliefs()
			if len(contracted) == 0 {
				fmt.Println("No contracted beliefs.")
			} else {
				fmt.Printf("%d contracted belief(s):\n", len(contracted))
				for _, id := range contracted {
					fmt.Printf("  %s\n", id)
				}
				fmt.Println("Use 'recover <id>' to reinstate after re-asserting the contracting record.")
			}
		case "recover":
			// recover <belief-id> — K÷5 recovery: reinstate a contracted belief
			if len(args) == 0 {
				fmt.Println("usage: recover <belief-id>")
				continue
			}
			if rErr := store.Recover(args[0], now); rErr != nil {
				fmt.Printf("recover error: %v\n", rErr)
			} else {
				fmt.Printf("Recovered: %s is now active.\n", args[0])
				lumen.SaveStore(store, db)
			}

		case "summary":
			s := store.Summarize(now)
			fmt.Print(lumen.RenderSummary(s))

		case "stats":
			beliefs := store.AllBeliefs(now)
			var nActive, nSuspect, nContracted int
			for _, b := range beliefs {
				switch b.State {
				case lumen.BeliefActive:    nActive++
				case lumen.BeliefSuspect:   nSuspect++
				case lumen.BeliefSuperseded: nContracted++
				}
			}
			fmt.Printf("Beliefs: %d (%d active, %d suspect, %d contracted)\n",
				len(beliefs), nActive, nSuspect, nContracted)
			fmt.Printf("Records: %d\n", store.RecordCount())
			gstats := store.Graph.Stats()
			var parts []string
			for k, v := range gstats { parts = append(parts, fmt.Sprintf("%s=%d", k, v)) }
			sort.Strings(parts)
			fmt.Printf("Graph:   %s\n", strings.Join(parts, " "))
			ec, mc := store.Entities.EntityStats()
			fmt.Printf("Entities: %d registered, %d mentions\n", ec, mc)
			tc := len(store.Temporal.Timeline())
			fmt.Printf("Temporal: %d events\n", tc)
		default:
			fmt.Printf("unknown command: %q — type 'help'\n", cmd)
		}
	}
}

func loadFile(s *lumen.Store, path string, now time.Time) error {
	if err := lumen.LoadFileWithImports(path, s, now); err != nil {
		return err
	}
	// Count only active beliefs (exclude soft-deleted contracted ones).
	active := 0
	for _, b := range s.AllBeliefs(now) {
		if b.State != lumen.BeliefSuperseded {
			active++
		}
	}
	queries := s.AllQueries()
	if len(queries) > 0 {
		fmt.Printf("Loaded %s — %d beliefs, %d named queries\n", path, active, len(queries))
	} else {
		fmt.Printf("Loaded %s — %d beliefs\n", path, active)
	}
	return nil
}

func loadAnalysis(s *lumen.Store, a *lumen.TextAnalysis, now time.Time) {
	for _, r := range a.Records {
		s.Assert(&lumen.Record{
			ID:        r.ID,
			Frame:     a.Frame,
			Content:   r.Content,
			Timestamp: now,
		})
	}
	for _, b := range a.Beliefs {
		s.Believe(&lumen.Belief{
			ID:         b.ID,
			Frame:      a.Frame,
			Content:    b.Content,
			Confidence: b.Confidence,
			AssertedAt: now,
			Derivation: b.DerivedFrom,
		})
	}
	fmt.Printf("Loaded %d records and %d beliefs from text analysis\n", len(a.Records), len(a.Beliefs))
}

func printHelp() {
	fmt.Print(`Lumen — epistemic belief store

File & persistence
  load <file.lm>            load a .lm belief file into the store
  analyze <file>            extract beliefs from free text and load
  save                      save current state to disk
  export [json|md]          export store in JSON or Markdown format
  exit / quit               save and exit

Beliefs
  list                      list all beliefs by confidence
  query <id>                current confidence, state, decay
  find <predicate>          predicate query — examples:
                              find confidence > 0.7
                              find frame = philosophical
                              find state = active
                              find content contains "consciousness"
                              find confidence >= 0.5 AND frame = empirical
  search <text>             TF-IDF semantic search
  stats                     store statistics

Provenance & explanation
  provenance <id>           full provenance chain with ⚑ foundational markers
  explain <id>              natural language epistemic explanation
  bio <id> [threshold]      epistemic biography: confidence arc, mind changes
  health <id>               health score with component breakdown (A–F grade)
  dot [file.dot]           export belief graph as Graphviz DOT
  assert <id> <frame> "<content>" [at DATE]   add a record
  believe <id> <frame> <conf> "<content>" [from r1,r2,...]  add a belief
  retract <id> [reason]     retract a record and show impact
  impact <id>               blast radius of retracting a record or belief
  fragility [n]             store-wide fragility scan — top n most vulnerable beliefs
  sensitivity <id>          source analysis: sorted by fragility, weakest link
  counterfactual <id> <src>  what-if: confidence without a specific source (alias: cf)
  contracted                 list contracted (soft-deleted) beliefs available for recovery
  recover <id>               K÷5 recovery: reinstate a contracted belief

Queries & time
  run <query-id>            execute a named query (from loaded .lm file)
  run <belief-id> <select>  ad-hoc query: confidence-changes | retraction-events | source-changes
                              options: since <date>  where change > 0.05
  queries                   list registered named queries

Epistemics
  timeline                  assertion ordering and temporal dependencies
  validate                  check store consistency (orphaned refs, cycles, frame refs)
  conflict                  scan for epistemic conflicts across beliefs
  calibrate                 flag calibration issues (over/under-confident beliefs)
  advance <duration>        advance the reference clock (e.g. "advance 30d")

  help                      this message
`)
}

func truncate(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n-3] + "..."
}

// BeliefHealthByID wraps Store.BeliefHealth for use by the REPL.
// Defined here rather than in the lumen package to keep the REPL self-contained.
func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("too short")
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return 0, err
	}
	switch s[len(s)-1] {
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	case 'm':
		return time.Duration(n) * 30 * 24 * time.Hour, nil
	case 'y':
		return time.Duration(n) * 365 * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("unknown unit %q (use d, w, m, y)", s[len(s)-1])
}
