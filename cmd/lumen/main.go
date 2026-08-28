package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	lumen "github.com/optakt/lumen"
)

var (
	store         = lumen.NewStore()
	now           = time.Now()
	composedCache = map[string]*lumen.ComposedBelief{}
	priorCache    = map[string]float64{}
	evidenceCache = map[string][]lumen.Evidence{}
)

func init() {
	// Register default frames so the REPL works out of the box
	for _, f := range []lumen.Frame{
		{Name: "default", Composition: lumen.CompositionBayesian,
			Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}, ProvenanceDepth: 3, ImportedDecayPolicy: "most_conservative"},
		{Name: "medical", Composition: lumen.CompositionBayesian,
			Decay:           lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 30 * 24 * time.Hour},
			ProvenanceDepth: 4, ImportedDecayPolicy: "most_conservative"},
		{Name: "sensor", Composition: lumen.CompositionBayesian,
			Decay:           lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: time.Hour},
			ProvenanceDepth: 2, ImportedDecayPolicy: "most_conservative"},
		{Name: "empirical", Composition: lumen.CompositionBayesian,
			Decay:           lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 10 * 365 * 24 * time.Hour},
			ProvenanceDepth: 5, ImportedDecayPolicy: "most_conservative"},
		{Name: "philosophical", Composition: lumen.CompositionBayesian,
			Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}, ProvenanceDepth: 5, ImportedDecayPolicy: "most_conservative"},
		{Name: "theoretical", Composition: lumen.CompositionBayesian,
			Decay:           lumen.DecayPolicy{Kind: lumen.DecayExponential, Halflife: 20 * 365 * 24 * time.Hour},
			ProvenanceDepth: 4, ImportedDecayPolicy: "most_conservative"},
		{Name: "reasoning", Composition: lumen.CompositionBayesian,
			Decay: lumen.DecayPolicy{Kind: lumen.DecayNone}, ProvenanceDepth: 5, ImportedDecayPolicy: "most_conservative"},
	} {
		store.RegisterFrame(f)
	}
}

func main() {
	if len(os.Args) > 1 {
		for _, path := range os.Args[1:] {
			src, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cannot read %s: %v\n", path, err)
				os.Exit(1)
			}
			if err := lumen.LoadFile(string(src), store, now); err != nil {
				fmt.Fprintf(os.Stderr, "load %s: %v\n", path, err)
				os.Exit(1)
			}
			fmt.Printf("loaded %s\n", path)
		}
	}

	fmt.Println("Lumen belief store REPL")
	fmt.Println("Commands: assert, believe, retract, query, trace, time, list,")
	fmt.Println("          compose, validate, sensitivity, audit, ds, help, quit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("lumen> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := handleCmd(line); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}
	fmt.Println("goodbye")
}

func handleCmd(line string) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}
	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "quit", "exit":
		fmt.Println("goodbye")
		os.Exit(0)

	case "help":
		fmt.Println(`Commands:
  assert <id> <frame> "<content>"       — add a record
  believe <id> <frame> <conf> [sources] "<content>" — add a belief
  retract <id> "<reason>"               — retract a record
  query <id>                            — current confidence
  trace <id>                            — epistemic trace
  list                                  — all beliefs
  time +<n>h|d|w|y                      — advance simulated time
  compose <id> prior:<f> <src>:lr:<f>[,conf:<f>] ... — add composed belief
  validate <id>                         — calibration check
  sensitivity <id>                      — marginal source contributions
  audit                                 — full audit report (all composed beliefs)
  ds <src1> T:<f>,F:<f>,?:<f> <src2> T:<f>,F:<f>,?:<f> — Dempster-Shafer combine
  load <path>                           — load a .lm file`)

	case "time":
		if len(args) == 0 {
			fmt.Println(now.Format(time.RFC3339))
			return nil
		}
		d, err := parseDuration(args[0])
		if err != nil {
			return err
		}
		now = now.Add(d)
		fmt.Printf("time: %s\n", now.Format(time.RFC3339))

	case "load":
		if len(args) == 0 {
			return fmt.Errorf("load requires a path")
		}
		src, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		if err := lumen.LoadFile(string(src), store, now); err != nil {
			return err
		}
		fmt.Printf("loaded %s\n", args[0])

	case "list":
		results := store.AllBeliefs(now)
		if len(results) == 0 {
			fmt.Println("(no beliefs)")
			return nil
		}
		for _, r := range results {
			flag := ""
			if r.State == lumen.BeliefSuspect {
				flag = " [RETRACTED]"
			}
			fmt.Printf("  %5.1f%%  [%-12s]  %s%s\n",
				r.CurrentConfidence*100, r.Frame, r.BeliefID, flag)
		}

	case "query":
		if len(args) == 0 {
			return fmt.Errorf("query requires an id")
		}
		r, err := store.Query(args[0], now)
		if err != nil {
			return err
		}
		fmt.Printf("%s: %.1f%% [%s]", args[0], r.CurrentConfidence*100, r.Frame)
		if r.State == lumen.BeliefSuspect {
			fmt.Print(" [RETRACTED]")
		}
		fmt.Println()

	case "trace":
		if len(args) == 0 {
			return fmt.Errorf("trace requires an id")
		}
		t, err := store.EpistemicTrace(args[0], now)
		if err != nil {
			return err
		}
		fmt.Println(t)

	case "assert":
		// assert <id> <frame> "<content>"
		if len(args) < 3 {
			return fmt.Errorf("assert <id> <frame> \"<content>\"")
		}
		content := strings.Join(args[2:], " ")
		content = strings.Trim(content, `"`)
		return store.Assert(&lumen.Record{
			ID: args[0], Frame: args[1], Content: content, Timestamp: now,
		})

	case "retract":
		if len(args) < 2 {
			return fmt.Errorf("retract <id> \"<reason>\"")
		}
		reason := strings.Join(args[1:], " ")
		reason = strings.Trim(reason, `"`)
		return store.Retract(args[0], reason, now)

	case "believe":
		// believe <id> <frame> <conf> [src,src,...] "<content>"
		if len(args) < 4 {
			return fmt.Errorf("believe <id> <frame> <conf> [sources] \"<content>\"")
		}
		conf, err := strconv.ParseFloat(args[2], 64)
		if err != nil {
			return fmt.Errorf("confidence: %v", err)
		}
		var derivation []string
		contentStart := 3
		if !strings.HasPrefix(args[3], `"`) {
			derivation = strings.Split(args[3], ",")
			contentStart = 4
		}
		content := strings.Join(args[contentStart:], " ")
		content = strings.Trim(content, `"`)
		return store.Believe(&lumen.Belief{
			ID: args[0], Frame: args[1], Confidence: conf,
			Content: content, AssertedAt: now, Derivation: derivation,
		})

	case "compose":
		// compose <id> prior:<f> <src>:lr:<f>[,conf:<f>] ... "<content>"
		if len(args) < 3 {
			return fmt.Errorf("compose <id> prior:<f> <src>:lr:<f> ... \"<content>\"")
		}
		id := args[0]
		prior, err := parseKV(args[1], "prior")
		if err != nil {
			return err
		}
		// Find content (last quoted arg)
		contentIdx := len(args) - 1
		for i := 2; i < len(args); i++ {
			if strings.HasPrefix(args[i], `"`) {
				contentIdx = i
				break
			}
		}
		content := strings.Join(args[contentIdx:], " ")
		content = strings.Trim(content, `"`)

		var evidence []lumen.Evidence
		for _, eArg := range args[2:contentIdx] {
			e, err := parseEvidenceArg(eArg)
			if err != nil {
				return err
			}
			evidence = append(evidence, e)
		}

		// Compute posterior first, then store with computed value as declared (no artificial discrepancy)
		computed, compErr := lumen.BayesianCompose(prior, evidence)
		if compErr != nil {
			return fmt.Errorf("composition: %v", compErr)
		}
		cb, err := store.BelieveComposed(&lumen.Belief{
			ID: id, Content: content, Confidence: computed,
			AssertedAt: now, Frame: "reasoning",
		}, prior, evidence)
		if err != nil {
			return err
		}
		composedCache[id] = cb
		priorCache[id] = prior
		evidenceCache[id] = evidence
		fmt.Println(lumen.FormatComposed(cb))

	case "validate":
		if len(args) == 0 {
			return fmt.Errorf("validate requires an id")
		}
		cb, ok := composedCache[args[0]]
		if !ok {
			return fmt.Errorf("no composition record for %s — use compose to add", args[0])
		}
		fmt.Println(lumen.FormatComposed(cb))

	case "sensitivity":
		if len(args) == 0 {
			return fmt.Errorf("sensitivity requires an id")
		}
		cb, ok := composedCache[args[0]]
		if !ok {
			return fmt.Errorf("no composition record for %s — use compose to add", args[0])
		}
		ev := evidenceCache[args[0]]
		prior := priorCache[args[0]]
		sens, err := lumen.SensitivityAnalysis(prior, ev, cb.ComputedConfidence)
		if err != nil {
			return err
		}
		fmt.Printf("Sensitivity for %s (prior=%.3f, posterior=%.3f):\n", args[0], prior, cb.ComputedConfidence)
		for _, s := range sens.Sources {
			dir := "↑"
			if s.MarginalContribution < 0 {
				dir = "↓"
			}
			fmt.Printf("  rank %d  %s %+.3f  %s (LR=%.1f conf=%.2f)\n",
				s.Rank, dir, s.MarginalContribution, s.SourceID, s.LikelihoodRatio, s.Confidence)
		}

	case "audit":
		var composed []*lumen.ComposedBelief
		for _, cb := range composedCache {
			composed = append(composed, cb)
		}
		if len(composed) == 0 {
			fmt.Println("(no composed beliefs — use 'compose' to add beliefs with evidence)")
			return nil
		}
		report := lumen.BuildAuditReport(composed, priorCache, evidenceCache)
		fmt.Println(lumen.FormatAuditReport(report))

	case "ds":
		// ds <src1> T:<f>,F:<f>,?:<f> <src2> T:<f>,F:<f>,?:<f>
		if len(args) < 4 {
			return fmt.Errorf("ds <src1> T:<f>,F:<f>,?:<f> <src2> T:<f>,F:<f>,?:<f>")
		}
		m1, err := parseDSMass(args[0], args[1])
		if err != nil {
			return fmt.Errorf("source 1: %v", err)
		}
		m2, err := parseDSMass(args[2], args[3])
		if err != nil {
			return fmt.Errorf("source 2: %v", err)
		}
		bel, pls, K, err := lumen.DempsterShaferCompose(m1, m2)
		if err != nil {
			return err
		}
		fmt.Printf("DS combination: %s × %s\n", args[0], args[2])
		fmt.Printf("  Belief: %.4f  Plausibility: %.4f  Conflict K: %.4f\n", bel, pls, K)
		fmt.Printf("  Uncertainty interval: [%.4f, %.4f]\n", bel, pls)
		if K > 0.5 {
			fmt.Printf("  ⚠ High conflict (K=%.3f) — sources make incompatible claims\n", K)
		}

	default:
		return fmt.Errorf("unknown command %q — type 'help'", cmd)
	}
	return nil
}

func parseKV(s, key string) (float64, error) {
	prefix := key + ":"
	if !strings.HasPrefix(s, prefix) {
		return 0, fmt.Errorf("expected %s:<float>, got %q", key, s)
	}
	return strconv.ParseFloat(s[len(prefix):], 64)
}

func parseEvidenceArg(s string) (lumen.Evidence, error) {
	// Format: <srcID>:lr:<float>[,conf:<float>]
	// The first token before any comma is srcID:lr:value
	// Subsequent comma-separated tokens are key:value pairs
	e := lumen.Evidence{Confidence: 1.0}
	parts := strings.Split(s, ",")
	if len(parts) == 0 {
		return e, fmt.Errorf("empty evidence arg")
	}
	// First part: srcID:lr:value
	first := parts[0]
	idx := strings.Index(first, ":lr:")
	if idx < 0 {
		return e, fmt.Errorf("invalid evidence arg %q — need <id>:lr:<float>", s)
	}
	e.SourceID = first[:idx]
	v, err := strconv.ParseFloat(first[idx+4:], 64)
	if err != nil {
		return e, fmt.Errorf("lr value for %s: %v", e.SourceID, err)
	}
	e.LikelihoodRatio = v
	// Remaining parts: key:value
	for _, p := range parts[1:] {
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			return e, fmt.Errorf("invalid key:value %q in evidence arg", p)
		}
		switch kv[0] {
		case "conf":
			c, err := strconv.ParseFloat(kv[1], 64)
			if err != nil {
				return e, fmt.Errorf("conf for %s: %v", e.SourceID, err)
			}
			e.Confidence = c
		}
	}
	if e.SourceID == "" || e.LikelihoodRatio == 0 {
		return e, fmt.Errorf("invalid evidence arg %q", s)
	}
	return e, nil
}

func parseDSMass(id, spec string) (lumen.DempsterShaferMass, error) {
	m := lumen.DempsterShaferMass{SourceID: id}
	for _, part := range strings.Split(spec, ",") {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			return m, fmt.Errorf("invalid mass spec %q", part)
		}
		v, err := strconv.ParseFloat(kv[1], 64)
		if err != nil {
			return m, fmt.Errorf("value for %s: %v", kv[0], err)
		}
		switch kv[0] {
		case "T":
			m.MassTrue = v
		case "F":
			m.MassFalse = v
		case "?":
			m.MassUnknown = v
		}
	}
	return m, nil
}

func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	sign := time.Duration(1)
	if s[0] == '+' {
		s = s[1:]
	} else if s[0] == '-' {
		sign = -1
		s = s[1:]
	}
	unit := s[len(s)-1]
	n, err := strconv.ParseFloat(s[:len(s)-1], 64)
	if err != nil {
		return 0, err
	}
	var dur time.Duration
	switch unit {
	case 'h':
		dur = time.Duration(n * float64(time.Hour))
	case 'd':
		dur = time.Duration(n * float64(24*time.Hour))
	case 'w':
		dur = time.Duration(n * float64(7*24*time.Hour))
	case 'y':
		dur = time.Duration(n * float64(365*24*time.Hour))
	default:
		return 0, fmt.Errorf("unknown unit %c", unit)
	}
	return sign * dur, nil
}
