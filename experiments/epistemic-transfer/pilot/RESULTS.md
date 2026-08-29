# Pilot Results

## Result

The corrected pilot identified four held-out model substrates from controlled belief-state transitions with **93.75% top-1 accuracy (15/16)** versus a **25% canonical-state null baseline (4/16)**.

| Representation | Correct | Accuracy |
|---|---:|---:|
| Canonical-state null baseline | 4/16 | 25.0% |
| Full intervention trajectory | 15/16 | 93.75% |
| Full trajectory, JSON-only compliance bit removed | 14/16 | 87.5% |
| Reference-conformance features only | 13/16 | 81.25% |
| Operator-summary features only | 11/16 | 68.75% |

Exploratory exact-binomial tail probability against 25% chance is `1.14e-8` for 15/16; the Wilson 95% interval is `[0.717, 0.989]`. These observations are not independent enough for that p-value to support a publication claim—the sample contains only four models, two runs, and paired structural variants—but the effect is large enough to justify a larger study.

## Experimental design

- Models: Claude Opus 4.6, GPT-5.6 Sol, Grok 4.6, DeepSeek V4 Pro
- Families: correlation disclosure, causal retraction cascade, retrodictive validity
- Variants: two synthetic domains and label sets per family
- Runs: two independent runs per model/episode
- Data: 48 complete trajectories, 128 model-generated transitions plus 48 canonical seed states
- Attribution: explainable nearest-centroid distance over typed trajectory features; no response embeddings, prose features, or LLM judge
- Evaluation: train on variant A and identify variant B, then reverse

Every conversation began from an identical, Lumen-injected canonical state. Only transitions after the first graph intervention came from the model.

The 25% null baseline is a construction, not an empirical static fingerprint: all four reference vectors are identical, and deterministic tie-breaking predicts the first model. A future study must compare against a genuine static behavioral-fingerprinting baseline on separate prompts.

## Critical methodological correction

The first execution let each model declare its own initial state. It produced 94% static identification and no improvement from intervention trajectories. That was not evidence for the hypothesis; it was a broken system-identification experiment because `B0` differed by model.

The runner was corrected to inject the episode reference state as the assistant's initial turn. After this change:

- static identification fell to chance: 25% in both cross-variant directions;
- full trajectories reached 87.5% in one direction and 100% in the other.

This correction is the strongest evidence that the measured signal comes from state transitions rather than initial prose or confidence style.

## Ablations

Cross-variant accuracies summed over both directions:

| Ablation | Correct | Accuracy |
|---|---:|---:|
| Correlation disclosure only | 15/16 | 93.75% |
| Retraction cascade only | 11/16 | 68.75% |
| Retrodictive validity only | 6/16 | 37.5% (4 tied cases skipped and counted incorrect) |

Correlation disclosure carries nearly all the pilot's signal. Retraction topology also carries model identity. The retrodiction episode is weak because it explicitly states the earlier posterior; models mostly retrieve the declared interval rather than reconstruct it.

## Observable signatures

### Correlation disclosure

Mean confidence reduction after revealing `r=0.85` evidence overlap:

| Model | Mean discount | Final midpoint |
|---|---:|---:|
| Claude Opus 4.6 | 0.0898 | 0.6837 |
| GPT-5.6 Sol | 0.0967 | 0.6819 |
| Grok 4.6 | 0.0960 | 0.6951 |
| DeepSeek V4 Pro | 0.1045 | 0.6714 |

The differences are numerically small but stable across changed source labels and domains. Full interval shape, intermediate updates, support sets, and reference residuals add separation beyond final midpoint alone.

### Retraction cascade

Opus, Sol, and Grok marked the focal belief suspect in all four cascade trajectories and marked all three dependent nodes suspect. DeepSeek marked the focal belief suspect in only one of four trajectories and averaged two suspect nodes. It often preserved a node with an independent surviving support path as active, despite the episode's declared Lumen semantics that any retracted ancestor makes the dependent suspect.

That is exactly the kind of epistemic-operator difference the pilot was designed to expose.

### Protocol behavior

Opus emitted prose outside the JSON object on arithmetic correlation steps, yielding 72.7% strict protocol compliance. The other three models were fully compliant. Removing the JSON-only compliance bit still produced 87.5% identification; field-level validity remains part of the epistemic response because malformed beliefs, states, and support sets are substantive operator behavior.

## Errors

One of sixteen held-out signatures was misidentified:

- one DeepSeek run as Grok when training on variant B.

It was a near-boundary case (distance gap `0.0082`).

## What this pilot establishes

It establishes feasibility, not the final claim:

> Under controlled initial state and held-out label/domain variants, a small set of formal belief-graph interventions produced stable enough model-specific transition residuals to identify four black-box models substantially above chance.

## What it does not establish

- Generalization to unseen graph topology
- Robustness across provider updates or inference configurations
- Open-set rejection of an unseen model
- Separation of model identity from provider-level decoding defaults; this pilot intentionally measured endpoint identity under each provider's default sampling, now recorded per result
- Superiority over strong static behavioral fingerprinting on a larger model panel
- Long-term stability of the signature
- A unique retrodiction fingerprint

## Next experiment

The next test should be harder, not merely larger:

1. Keep `B0` canonical and external.
2. Generate at least four graph topologies per intervention family.
3. Hold out an entire topology, not just names and domain prose.
4. Remove declared posterior values from retrodiction episodes; require reconstruction from timestamped evidence.
5. Add source-reliability reversal and recovery hysteresis.
6. Include all ten Pilot 1 models and an unseen eleventh model for open-set rejection.
7. Compare against LLMmap-style static prompts and an output-embedding classifier.
8. Report provider parameters as part of the endpoint identity.

The pilot passes the threshold for continuing. It does not yet pass the threshold for publication.
