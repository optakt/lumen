# Design Notes

Architecture decisions and the reasoning behind them.

## Self-Model Architecture

The `self` package and the main belief store are intentionally separate.

**`self` package** — a standalone epistemic session for claims *about the agent*. Typed claim kinds (`asserted`, `derived`, `retrieved`, `corrected`) map to frames with appropriate decay policies. The package has no dependency on the main store; it runs as a pure in-process library.

**Main store** — a belief store for claims *about the world*, with full provenance, graph structure, persistence, and analysis tooling.

**The bridge is the HTTP server.** `POST /v1/self/claim` stores self-model claims as regular `Belief` objects in the main store with a `self:` ID prefix and `sentinel:` records for cascade retraction. This means:

- Self-model claims survive BoltDB round-trips automatically.
- `GET /v1/self/context` filters beliefs by the `self:` prefix.
- `EpistemicBiography` works on self-model claims via their belief IDs.
- FragilityScan and ImpactScan apply to self-model claims alongside world-beliefs.

This design avoids a second persistence layer and keeps the analysis tools unified. The separation at the package level means the `self` library can be embedded in contexts without the HTTP server (e.g., the REPL, the debate simulator).

---

## norScale vs Sensitivity Analysis in FragilityScan

`FragilityScan` uses two paths depending on what metadata is available:

**Sensitivity analysis (exact)** — when a belief was created via `BelieveComposed`, the prior and evidence blocks are stored on the `Belief` struct. FragilityScan uses `SensitivityAnalysis` to recompute the Bayesian posterior with each evidence source removed. This is exact given the stored prior and evidence.

**Proportional decay approximation** — for beliefs derived via `Believe` (no Bayesian composition), confidence is modelled as `NoisyOr(sources) × decay_factor`, where `decay_factor = current / NoisyOr(all sources)`. Removing source *k* gives:

```
estimated = NoisyOr(sources \ {k}) / NoisyOr(sources) × current
```

This is implemented by `norScale`. It is principled under two assumptions:
1. Confidence is monotonically proportional to the noisy-or of sources.
2. The decay factor is uniform — it applies to the belief over time, not per source.

Both hold in practice: Lumen's decay applies to the whole belief from its assertion time, not to individual sources. The approximation breaks if a belief has a dominant prior not captured in its derivation list, or if sources are strongly correlated (handle with `BayesianComposeCorrelated` instead).

---

## Retrodiction Fix

The core design insight that motivated Lumen: cross-frame belief import should snapshot confidence at assertion time, not apply ongoing foreign-frame decay.

**Problem:** If belief B1 in frame F1 (halflife 1 year) is imported into frame F2 (halflife 1 week), naive import applies F1's ongoing decay to B1 inside F2. Thirty days later, B1 appears to have decayed to near-zero — not because F2's reasoning has become stale, but because F1's original halflife accumulated against the wrong start time.

**Fix:** `CrossFrameSource` snapshots the source belief's confidence at the crossing time. Inside F2, only F2's decay policy applies, starting from when the cross-frame assertion was made.

This was discovered during development as a live design flaw in the running implementation.

---

## Evidence Persistence

`BelieveComposed` stores the prior and evidence blocks directly on the `Belief` struct (`CompositionPrior`, `CompositionEvidence`). Since `SaveStore` serialises the full `Belief` struct to BoltDB via JSON, composition metadata survives restarts without a separate bucket.

The alternative — a separate `composed_beliefs` bucket — was rejected because it would require coordination on every read path that accesses belief state.
