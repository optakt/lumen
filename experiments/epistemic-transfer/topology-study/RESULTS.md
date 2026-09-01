# Topology-Held-Out Study Results

## Verdict

The study **does not support superiority over response texture or reliable open-set identification**. It does support a different, above-chance closed-set identification signal from epistemic transitions.

Controlled belief-revision trajectories contain stable model-specific signal and identify eight known black-box endpoints on unseen graph topologies with **76.56% top-1 accuracy**. However, a directly comparable final-response hashed-texture baseline reaches **75.00%**. The paired difference is negligible and non-significant (`McNemar exact p=0.8804`).

Without the hashed-texture representation, probability trajectories reach **64.84%**, graph/state trajectories **58.59%**, and Lumen-native operator summaries **28.91%**, all above the eight-class chance rate of 12.5%. Each intervention family alone reaches 42.19–46.88%. Open-set rejection fails: the calibrated matcher rejects **0/16** Haiku signatures, although distance ranking has AUROC `0.7612`.

Under the predeclared decision criteria, the claim that formal belief-graph transition geometry provides a uniquely stronger, open-set-capable fingerprint must be abandoned. The study did not directly remove or normalize response texture, so robustness to that intervention remains untested rather than disproved.

## Corpus

- 8 complete known endpoints
- 1 open-set endpoint (Claude Haiku 4.5)
- 5 intervention families
- 4 graph topologies held out in turn
- 2 surface variants
- 2 runs
- 720 complete trajectories
- 2,160 model-generated transitions
- 720 canonical seed states

Known endpoints:

- Claude Opus 4.6
- Claude Sonnet 4.6
- GPT-5.6 Sol
- Grok 4.6
- DeepSeek V4 Pro
- Kimi K3 (explicit non-thinking endpoint configuration)
- GLM 5.3 (explicit non-thinking endpoint configuration)
- Gemini 3.7 Flash

Qwen 3.8 Max exhausted its weekly token-plan quota. Later protocol hardening invalidated nearly all earlier acquisitions; only 3 complete trajectories match the final protocol. Qwen is excluded rather than imputed. Fable 5 was unavailable due exhausted credits.

## Closed-set topology-held-out attribution

Chance is 12.5% for eight classes.

| Representation | Correct | Accuracy | Skipped |
|---|---:|---:|---:|
| Final structured state | 87/128 | 67.97% | 0 |
| Final hashed response texture | 96/128 | 75.00% | 0 |
| Probability trajectory | 83/128 | 64.84% | 0 |
| Graph/state trajectory | 75/128 | 58.59% | 2 |
| Operator summaries | 37/128 | 28.91% | 0 |
| **Full Lumen graph-operator signature** | **98/128** | **76.56%** | **0** |

Full-signature folds:

| Held-out topology | Correct | Accuracy |
|---|---:|---:|
| Chain | 21/32 | 65.62% |
| Fork | 27/32 | 84.38% |
| Diamond | 28/32 | 87.50% |
| Mesh | 22/32 | 68.75% |

## Paired tests

The same 128 signatures are classified by every representation.

| Baseline | Full correct / baseline wrong | Baseline correct / full wrong | Exact McNemar p |
|---|---:|---:|---:|
| Final structured | 17 | 6 | 0.0347 |
| Final texture | 23 | 21 | **0.8804** |
| Probability trajectory | 30 | 15 | 0.0357 |
| Graph/state | 28 | 5 | 0.0001 |
| Operator summaries | 68 | 7 | <0.0001 |

The full representation beats structured numeric/state baselines. It does not beat ordinary final-output texture.

Wilson 95% intervals overlap heavily: full signature `[68.52%, 83.06%]`; final texture `[66.84%, 81.70%]`. The McNemar result is a failure to demonstrate superiority, not proof of exact equivalence. Five paired comparisons were reported without multiplicity correction; under Bonferroni, the subsidiary improvements over final structured and probability trajectories are no longer significant. The headline full-versus-texture result is unaffected.

## Intervention-family attribution

Using each family's full trajectory in isolation:

| Family | Correct | Accuracy |
|---|---:|---:|
| Correlation disclosure | 60/128 | 46.88% |
| Retraction cascade | 58/128 | 45.31% |
| Retrodictive validity | 54/128 | 42.19% |
| Source-reliability reversal | 60/128 | 46.88% (3 ties counted wrong) |
| Recovery hysteresis | 58/128 | 45.31% |

Every family carries above-chance identity signal, but no single Lumen operator produces a strong standalone fingerprint.

## Distance structure

- Within-model median distance: `0.027973` (`n=960`)
- Between-model median distance under matched topology/variant/run: `0.066051` (`n=448`)
- Pairwise AUROC, between distance greater than within distance: `0.8421`

This is the positive result: epistemic trajectories are measurably more stable within an endpoint than between endpoints.

## Open-set failure

Thresholds were calibrated at the 95th percentile using known models and training topologies only.

- Known acceptance: 119/128 = 92.97%
- Haiku rejection: 0/16 = 0.00%
- Distance AUROC: 0.7612

Leave-one-known-model-out rejection is also weak:

- 24/128 = 18.75% rejected overall
- DeepSeek is the only endpoint reliably rejected when unseen (16/16)

The signature is useful for closed-set similarity, not reliable unknown-model detection.

## Separate Pilot 1 static baselines

These were acquired earlier on 14 identical static probes and evaluated leave-one-run-out. They are not topology-held-out and are reported separately:

- Opinion/confidence: 23/24 = 95.83%
- Hashed response texture: 24/24 = 100.00%

They show that ordinary static behavior identifies these endpoints more accurately in its easier acquisition regime. They do not test whether epistemic-transition fingerprints survive deliberate removal of response texture.

## Design corrections made before final acquisition

The final corpus incorporates several rejected intermediate designs:

1. **Canonical B0** — model-generated initial states created a false static ceiling in the pilot.
2. **No anticipatory intervention leakage** — correlation graphs, future retractions, and future recovery records are withheld until their intervention turn.
3. **Primitive controls** — likelihood ratios, source confidences, and correlation edge strengths are held constant across topology folds; only connectivity varies.
4. **Topology-level split** — train on three graph topologies, test on the fourth.
5. **Response-configuration fingerprints** — episode SHA-256 and response-affecting provider configuration must match before a trajectory can be reused.
6. **No partial scoring** — incomplete trajectories, ties, and incompatible schemas cannot count as correct.
7. **Nested open-set calibration** — thresholds use known models and training topologies only.
8. **Qwen exclusion** — quota-limited missing data is not imputed.

## What remains valuable

The model-identification mechanism is different, but was not shown to be more accurate than response texture or robust after texture removal. Lumen exposes interpretable differences that a text classifier cannot explain:

- confidence response to correlated evidence;
- propagation choices after causal retraction;
- reliability upgrade/downgrade asymmetry;
- retrodictive preservation versus presentism;
- recovery hysteresis and node-state restoration.

The 0.8421 within/between distance AUROC suggests a possible use as an **epistemic behavior audit** or endpoint-drift monitor. That direction overlaps existing behavioral monitoring work and should not be pursued as a novelty claim without a separate reason.

## Decision

Stop expanding the model-fingerprinting benchmark.

Do not tune feature weights, thresholds, or episode selection against this test set. Doing so would convert a clean comparative result into overfitting. Preserve the corpus and analysis as evidence that the superiority and open-set hypotheses failed, while distinct closed-set epistemic-transition signal remained.
