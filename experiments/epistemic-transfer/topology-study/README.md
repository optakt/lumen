# Topology-Held-Out Epistemic System Identification

This study asks a harder question than the four-model feasibility pilot:

> Does a model-specific belief-revision signature generalize to an unseen graph topology, rather than merely to renamed entities and prose?

## Corpus

The deterministic generator creates 40 executable `.lm` episodes:

- 5 intervention families
  - correlation disclosure
  - causal retraction cascade
  - retrodictive validity
  - source-reliability reversal
  - recovery hysteresis
- 4 graph topologies: `chain`, `fork`, `diamond`, `mesh`
- 2 independently labeled surface variants per family/topology

Every family uses the same step-role schema across topologies. Names and synthetic domains change, but feature keys do not.

Generate the corpus:

```bash
go run ../../../cmd/generate-transfer-study -out episodes
```

## Model panel

Eight complete known frontier endpoints:

- Claude Opus 4.6
- Claude Sonnet 4.6
- GPT-5.6 Sol
- Grok 4.6
- DeepSeek V4 Pro
- Kimi K3
- GLM 5.3
- Gemini 3.7 Flash

Fable 5 is omitted while credits are unavailable. Qwen 3.8 Max exhausted its weekly token-plan quota during acquisition. Later protocol hardening invalidated nearly all earlier trajectories; only 3 complete trajectories match the final protocol. Qwen is excluded from headline attribution rather than imputed (`qwen-excluded.jsonl`). Claude Haiku 4.5 is acquired as an open-set endpoint only; it never contributes a known-class centroid.

Sampling defaults are deliberately preserved. Output ceilings, thinking controls, timeouts, and concurrency are provider-specific operational settings recorded with every result. The study therefore identifies deployed endpoint configurations, not architecture in isolation.

## Acquisition

```bash
./run.sh 2 113
```

Two runs produce:

- 720 complete study trajectories total
- 640 known-class trajectories
- 80 open-set trajectories
- 2,160 model-generated transitions, plus 720 canonical seed states

Acquisition is resumable. Every result records the exact provider configuration, parser version, episode SHA-256, model, run, and timestamp.

## Evaluation

```bash
./analyze.sh
```

For each held-out topology:

1. Build model centroids from the other three topologies.
2. Identify every signature on the unseen topology.
3. Repeat for all four folds.

Representations:

- final-state structured endpoint baseline
- final-state hashed texture baseline on the same held-out topology
- probability-trajectory baseline
- graph/state trajectory baseline
- operator summaries
- full Lumen graph-operator signature

Separate Pilot 1 static baselines report leave-one-run-out attribution from opinion/confidence and hashed response texture. They are a different acquisition and are labeled accordingly.

## Open-set rejection

Claude Haiku is absent from all known centroids. For each topology fold, the rejection threshold is calibrated only on known models and training topologies. The study reports both known acceptance and unknown rejection.

## Comparative decision criteria

The benchmark stops if any of the following occur; these criteria test superiority and open-set usefulness, not whether trajectories carry any model-specific signal:

- full graph-operator signatures do not beat final-state and probability-only baselines on held-out topologies;
- within-model variation approaches between-model variation;
- open-set rejection fails;
- correlation and graph operators add no signal beyond probability trajectories;
- accuracy depends on missing results, parser compliance, or provider failures.

No publication claim should be made from this study until acquisition is complete, all trajectory schemas are identical, and an independent review confirms the split and thresholds are leakage-free.
