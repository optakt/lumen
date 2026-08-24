package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	lumen "github.com/optakt/lumen"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: audit <file.lm>")
		os.Exit(1)
	}
	path := os.Args[1]
	now := time.Now()

	store := lumen.NewStore()
	frames := []lumen.Frame{
		{Name: "philosophical", Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}},
		{Name: "empirical", Decay: lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 5 * 365 * 24 * time.Hour}},
		{Name: "contemporary", Decay: lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 10 * 365 * 24 * time.Hour}},
		{Name: "reasoning", Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}},
	}
	for _, f := range frames {
		store.RegisterFrame(f)
	}

	if err := lumen.LoadFileWithImports(path, store, now); err != nil {
		fmt.Fprintf(os.Stderr, "load error: %v\n", err)
		os.Exit(1)
	}

	// Bootstrap entity graph: extract candidate named entities from all belief content
	// using SimpleNER, register them, then re-index all beliefs for co-mention analysis.
	beliefs := store.AllBeliefs(now)
	var allContent []string
	for _, b := range beliefs {
		allContent = append(allContent, b.Content)
	}
	candidates := make(map[string]bool)
	for _, content := range allContent {
		for _, c := range lumen.SimpleNER(content) {
			candidates[c] = true
		}
	}
	for c := range candidates {
		store.Entities.RegisterEntity(&lumen.Entity{ID: c})
	}
	// Re-index all beliefs against registered entities
	for _, b := range beliefs {
		store.Entities.ExtractAndIndex(b.BeliefID, b.Content)
	}

	fmt.Printf("=== Epistemic Audit: %s ===\n\n", path)
	fmt.Printf("Loaded: %d beliefs\n", len(beliefs))

	graphStats := store.Graph.Stats()
	parts := []string{}
	for kind, count := range graphStats {
		parts = append(parts, fmt.Sprintf("%s=%d", kind, count))
	}
	sort.Strings(parts)
	fmt.Printf("Graph: %s\n", strings.Join(parts, " "))

	entityCount, mentionCount := store.Entities.EntityStats()
	fmt.Printf("Entities: %d registered, %d mentions\n\n", entityCount, mentionCount)

	var suspect []lumen.QueryResult
	for _, b := range beliefs {
		if b.State == lumen.BeliefSuspect {
			suspect = append(suspect, *b)
		}
	}
	if len(suspect) > 0 {
		fmt.Printf("--- SUSPECT BELIEFS (%d) ---\n", len(suspect))
		for _, b := range suspect {
			fmt.Printf("  [%s] %.0f%% %q\n", b.Frame, b.CurrentConfidence*100, trunc(b.Content, 60))
		}
		fmt.Println()
	}

	sort.Slice(beliefs, func(i, j int) bool {
		return beliefs[i].CurrentConfidence < beliefs[j].CurrentConfidence
	})
	fmt.Printf("--- CONFIDENCE DISTRIBUTION ---\n")
	buckets := [4]int{}
	for _, b := range beliefs {
		c := b.CurrentConfidence
		switch {
		case c < 0.5:
			buckets[0]++
		case c < 0.7:
			buckets[1]++
		case c < 0.9:
			buckets[2]++
		default:
			buckets[3]++
		}
	}
	fmt.Printf("  <50%%: %d  50-70%%: %d  70-90%%: %d  >90%%: %d\n\n",
		buckets[0], buckets[1], buckets[2], buckets[3])

	fmt.Printf("--- ENTITY CO-MENTION CLUSTERS ---\n")
	seen := make(map[string]bool)
	clusterCount := 0
	for _, b := range beliefs {
		if seen[b.BeliefID] {
			continue
		}
		co := store.Entities.CoMentioned(b.BeliefID, 1)
		if len(co) >= 1 {
			seen[b.BeliefID] = true
			peerIDs := make([]string, len(co))
			for i, c := range co {
				peerIDs[i] = c.NodeID
				seen[c.NodeID] = true
			}
			fmt.Printf("  Hub: %q\n  Peers (%d): %s\n\n",
				trunc(b.Content, 60), len(peerIDs), strings.Join(peerIDs, ", "))
			clusterCount++
		}
	}
	if clusterCount == 0 {
		fmt.Printf("  (no strong co-mention clusters — try registering entities)\n\n")
	}

	fmt.Printf("--- DECAY REPORT ---\n")
	var decayed []lumen.QueryResult
	for _, b := range beliefs {
		if now.Sub(b.AssertedAt) > 24*time.Hour && b.CurrentConfidence < 0.8 {
			decayed = append(decayed, *b)
		}
	}
	if len(decayed) == 0 {
		fmt.Printf("  (no significant decay)\n")
	} else {
		sort.Slice(decayed, func(i, j int) bool {
			return decayed[i].CurrentConfidence < decayed[j].CurrentConfidence
		})
		for _, b := range decayed {
			fmt.Printf("  %.0f%% [%s] %q\n", b.CurrentConfidence*100, b.Frame, trunc(b.Content, 60))
		}
	}
	fmt.Printf("\n=== Audit complete ===\n")
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
