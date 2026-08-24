# Lumen

A belief store for reasoning under uncertainty. Lumen tracks what an agent believes, how confident it is, why, and how that changes over time.

## The problem

AI agents accumulate claims across a conversation: facts retrieved from memory, conclusions drawn from evidence, inferences stacked on inferences. Most systems treat this as a flat context window — no structure, no provenance, no way to ask "if this source turns out to be wrong, what else breaks?"

Lumen gives beliefs structure: typed frames with decay policies, derivation chains, retraction cascades, and AGM-compliant belief revision. When a source is retracted, every belief that depended on it is automatically flagged. When evidence accumulates, Bayesian composition tracks how posterior confidence should shift.

## The retrodiction problem

Standard cross-frame belief import has a subtle bug: when a belief from frame A is used as evidence in frame B, which frame's decay clock applies going forward?

Using frame A's decay (the source's) produces the *retrodiction problem* — a month-old sensor reading incorporated into a diagnosis decays as if the reading were being taken continuously, voiding the historical evidence. The correct answer: snapshot the source's confidence at import time, then decay it only under the receiving frame's policy. Lumen fixes this.

## Quick start

```bash
git clone https://github.com/optakt/lumen
cd lumen
go build ./cmd/lumen-db

# Run the interactive belief store
./lumen-db examples/consciousness.lm
```

Inside the REPL:
```
> list                         # all active beliefs with confidence
> query hard-problem           # current state of a belief
> explain hard-problem         # why this confidence? what sources?
> fragility                    # which beliefs would collapse first?
> impact chalmers-1995         # what breaks if this record is retracted?
> bio gwt-strong               # full epistemic history of a belief
> dot graph.dot                # export belief graph (render with graphviz)
> validate                     # check store consistency
```

## The .lm format

Beliefs are declared in `.lm` files:

```lm
frame empirical
    decay: exponential halflife: 5y

frame philosophical
    decay: none

record chalmers-1995 in philosophical
    content: "The hard problem of consciousness is real."
    timestamp: "1995-01-01"

record cogitate-2023 in empirical
    content: "Cogitate Consortium: prefrontal cortex not necessary for consciousness."
    timestamp: "2023-06-01"

believe hard-problem in philosophical
    confidence: 0.78
    content: "The hard problem of consciousness is real."
    from: chalmers-1995, jackson-1982, zombie-argument, conscious-experience-exists

query recent-gwt-changes
    target: gwt-strong
    select: confidence-changes
    since: "2023-01-01"
    where: change > 0.05
```

Files are round-trippable: `export lm` produces valid `.lm` syntax that re-imports cleanly.

## Core concepts

**Record** — an immutable source of evidence: a paper, a sensor reading, an observation, a conversation. Retracting a record cascades suspect-marking to all beliefs that derived from it.

**Belief** — a claim with a confidence value, a frame, and a derivation chain. Confidence decays according to the frame's policy. Beliefs derive from records and other beliefs.

**Frame** — a named epistemic context with a decay policy (`none`, `exponential halflife: Ny`, `step after: Ny`). Evidence from one frame imported into another snapshots at the moment of import; only the receiving frame's clock applies thereafter.

**Bridge** — a declared translation between incompatible frames, with explicit loss annotation and assumption tracking.

## AGM compliance

Lumen implements the AGM belief revision postulates:

- **K÷1–K÷6** (Contraction): `MinimalContraction` computes the minimal belief set that removes a retracted record while preserving maximum coherent structure. `ApplyContraction` soft-deletes contracted beliefs, enabling K÷5 recovery.
- **K\*1–K\*6** (Revision): `Revise` updates a record and marks dependents suspect for re-evaluation.
- **Recovery (K÷5)**: `Recover` restores a contracted belief when its contracting record is re-asserted.

## Bayesian composition

```go
cb, err := store.BelieveComposed(&Belief{
    ID: "diagnosis", Frame: "clinical",
    Content: "Patient has hypertension.",
    Confidence: 0.72,
    Derivation: []string{"bp-high", "family-history"},
}, 0.30, []Evidence{
    {SourceID: "bp-high",        LikelihoodRatio: 5.0, Confidence: 0.85},
    {SourceID: "family-history", LikelihoodRatio: 3.0, Confidence: 0.70},
})
// cb.Discrepancy shows gap between declared and computed confidence
// cb.Calibration flags overconfidence or underconfidence
```

Correlation-aware composition prevents double-counting when evidence shares underlying structure (e.g., zombie argument, knowledge argument, and conceivability argument all run on the same modal intuition). Credal sets (interval priors and likelihood ratios) propagate epistemic uncertainty end-to-end.

## API

```go
import "github.com/optakt/lumen"

s := lumen.NewStore()
s.RegisterFrame(lumen.Frame{
    Name:  "empirical",
    Decay: lumen.DecayPolicy{Kind: "exponential", Halflife: 5 * 365 * 24 * time.Hour},
})

// Assert a record
s.Assert(&lumen.Record{ID: "r1", Frame: "empirical", Content: "...", Timestamp: time.Now()})

// Believe something derived from it
s.Believe(&lumen.Belief{
    ID: "b1", Frame: "empirical",
    Content: "...", Confidence: 0.80,
    AssertedAt: time.Now(), Derivation: []string{"r1"},
})

// Query with decay applied
result, _ := s.Query("b1", time.Now())
// result.CurrentConfidence — decayed confidence at query time
// result.State             — Active, Suspect, or Superseded

// Retract a source — cascades to all dependent beliefs
s.Retract("r1", "paper retracted", time.Now())

// What breaks if r1 is retracted?
entries, _ := s.ImpactScan("r1", time.Now())

// Which beliefs are most fragile?
fragile := s.FragilityScan(time.Now())

// Epistemic biography of a belief
bio, _ := s.EpistemicBiography("b1", 0.05, time.Now())

// Export for round-trip or inspection
lm := s.ExportLM(time.Now())
dot := s.ExportDot(lumen.DefaultDotOptions(time.Now()))
```

## Persistence

```go
db, _ := lumen.OpenDB("store.db")
defer db.Close()
lumen.SaveStore(store, db)

// Later
store, _ = lumen.LoadStore(db, time.Now())
```

## Four graphs

Lumen maintains four internal graphs over beliefs and records:

1. **BeliefGraph** — typed derivation and semantic edges; single source of truth for retraction cascades
2. **EntityGraph** — bipartite graph between beliefs/records and named entities; enables "what does the store know about X?"
3. **TemporalGraph** — assertion ordering and historical state; enables "what did the store believe before record R was asserted?"
4. **BridgeRegistry** — declared frame-to-frame translation protocols with cumulative loss tracking

## lumen-db commands

```
load <file>          Load a .lm file
list                 Active beliefs ranked by confidence
query <id>           Current belief state
explain <id>         Natural language explanation with source attribution
bio <id> [thresh]    Epistemic biography: revisions, decay trajectory, retractions
provenance <id>      Full provenance chain (⚑ marks foundational records)
health <id>          Epistemic health score (A–F)
sensitivity <id>     Which source removal hurts most
fragility [n]        Store-wide fragility ranking
impact <id>          Blast radius of retracting a record or belief
validate             Consistency check (orphaned refs, cycles, undefined frames)
conflict             Scan for epistemic conflicts
calibrate            Flag suspect, low-confidence, and stale-deriver beliefs
summary              High-level epistemic snapshot with per-frame statistics
assert <id> <frame> "<content>" [at DATE]
believe <id> <frame> <conf> "<content>" [from r1,r2,...]
retract <id> [reason]
run <id>             Execute a named query
find <predicate>     Predicate query (confidence > 0.7 AND frame = empirical)
search <terms>       TF-IDF search over belief content
export [json|lm|md]  Export store
dot [file.dot]       Export belief graph (render: dot -Tsvg file.dot -o graph.svg)
advance <duration>   Move reference clock for decay testing (or: advance reset)
```

## Status

Research prototype. The core epistemic model is complete and tested (223 tests). Not production-hardened: no multi-writer support, Bayesian composition metadata is not persisted across restarts, the text extraction pipeline is heuristic.

Apache 2.0 licensed.
