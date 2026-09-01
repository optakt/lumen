# Epistemic System Identification — Novelty Review

**Research date:** 2026-08-28  
**Question:** Is identifying an LLM from its controlled belief-revision dynamics a genuinely new use of Lumen, rather than ordinary model fingerprinting with epistemic vocabulary?

## Verdict

The broad idea is **not new**:

- Identifying LLMs from black-box outputs is established.
- Identifying models from structured evaluative dispositions is established.
- Measuring sequential belief trajectories against Bayesian references is established.
- Measuring model-specific degradation curves under controlled perturbations is established.

The narrower Lumen-native mechanism appears **unclaimed in the literature reviewed**:

> Treat a black-box LLM as an epistemic state-transition operator; intervene on a formally declared belief graph with correlation disclosures, source-reliability reversals, AGM contractions, causal retraction cascades, lossy frame bridges, historical retrodiction queries, and evidence recovery; then identify the model from the residual geometry of its belief biography relative to the declared Lumen semantics.

No paper found combines **model attribution** with **multi-operation belief-graph intervention trajectories**. No paper found uses retraction cascade topology, cross-frame loss, retrodictive validity, or recovery hysteresis as model-identification signals.

That is a credible novelty claim, not yet a guaranteed one. A submission would still need conventional citation chasing and an updated scholarly-index search immediately before claiming priority.

## What We Cannot Claim

### “Models can be identified from a few behavioral prompts”

LLMmap identifies 42 versions at over 95% accuracy with 3–8 active queries, including under unknown system prompts, stochastic sampling, RAG, and chain-of-thought configurations.

### “Models have stable behavioral fingerprints”

Behavioral Fingerprinting profiles 18 models across reasoning, metacognition, sycophancy, robustness, and personality-like dimensions. Natural Fingerprints shows that even identical data and architecture can yield distinguishable outputs from optimization settings, data order, and random seed.

### “Structured judgments reveal model identity”

Evaluative Fingerprints identifies nine LLM judges with 77.1% accuracy from five rubric scores and 89.9% from scores plus disposition features. GPT-4.1 and GPT-5.2 are distinguished with 99.6% accuracy. The fingerprint transfers across domains and survives prompt perturbation.

This is the nearest attribution paper. Our work must distinguish belief-state transitions from evaluative score patterns.

### “Belief trajectories expose model differences”

BayesBench already scores per-turn belief trajectories under multi-turn evidence accumulation. Chen et al. treat LLMs as information-processing rules and measure their deviation from Bayesian updating. Farmer et al. fit reproducible log-ratio probability transformations across models and prompting configurations.

Trajectory measurement alone is not novel.

### “Controlled perturbation curves characterize models”

Structured-perturbation work already measures per-family degradation curves, stability, and collapse points across graded stressors. Stability Monitor fingerprints deployed endpoints from response distributions and detects changes to model version, inference stack, quantization, and provider environment.

Calling our output a “transfer function” is not enough. The intervention algebra must be epistemically distinct.

## Nearest Work by Research Line

### 1. Black-box model fingerprinting

| Work | Method | Result | Difference from proposed work |
|---|---|---|---|
| [LLMmap](https://arxiv.org/abs/2407.15847) | Carefully chosen natural-language queries; response classifier | >95% over 42 versions with 3–8 interactions | Static response identity, not belief revision |
| [Hide and Seek](https://arxiv.org/abs/2408.02871) | LLM-designed discriminative prompts and response analysis | 72% family attribution | Adaptive prompt discovery, not formal epistemic state |
| [Natural Fingerprints](https://arxiv.org/abs/2504.14871) | Source-model classifier over generated text under controlled training conditions | Training dynamics and random seed remain identifiable | Explains origin of output signatures, not belief updates |
| [RAFP](https://arxiv.org/abs/2505.12682) | Rare-region prompt fingerprints robust to finetuning | Lineage identification | IP/lineage focus; no interpretable epistemic operation |
| [Behavioral Fingerprints for Endpoint Stability](https://arxiv.org/abs/2603.19022) | Fixed prompts, response embeddings, energy distance, sequential change detection | Detects family, version, stack, quantization, provider shifts | Monitors distribution shift rather than identifying revision operator |
| [Fingerprinting Inference Systems](https://arxiv.org/abs/2605.29979) | Exploits numerical deviations propagated to text | Identifies inference-system components | Lower-level systems fingerprint, not beliefs |

### 2. Behavioral and evaluative fingerprints

| Work | Method | Result | Difference |
|---|---|---|---|
| [Behavioral Fingerprinting](https://arxiv.org/abs/2509.04504) | 21 static diagnostic prompts; LLM judge scores reasoning, metacognition, bias, robustness, personality analogue | Distinct radar profiles across 18 models | Static, LLM-judged, no attribution experiment, no state transition |
| [Evaluative Fingerprints](https://arxiv.org/abs/2601.05114) | Models evaluate fixed artifacts on five rubric dimensions; random-forest attribution | 89.9% exact judge; 99.6% within GPT family | Strongest overlap: identity from structured dispositions, but artifacts do not alter a model-held belief graph |
| [Behavioral Shift Auditing](https://arxiv.org/abs/2410.19406) | Hypothesis test comparing generation distributions | Detects post-deployment behavior shifts with false-positive control | Detects change, not epistemic mechanism or closed-set identity |
| [Structured Perturbations](https://arxiv.org/abs/2608.22138) | Graded paraphrase/noise/format/context/conflict/unanswerability stress | Per-family degradation and collapse profiles | Perturbs task inputs; does not manipulate source provenance or belief dependencies |

### 3. Belief revision and sequential evidence

| Work | Method | Result | Difference |
|---|---|---|---|
| [Belief-R](https://aclanthology.org/2024.emnlp-main.586/) | Two-step delta reasoning: initial inference, then premise addition requiring update or stay | Models trade off correct updating against correct preservation | Logical conclusion revision only; no probabilistic trajectory or attribution |
| [Rational Belief Revision / Model Editing](https://arxiv.org/abs/2406.19354) | Semi-synthetic Wikidata world; exact Bayesian reference | Quantifies model-edit divergence from Bayesian posterior | Weight editing and probabilistic coherence, not black-box episodic identity |
| [Bayesian Coherence Coefficient](https://arxiv.org/abs/2507.17951) | Compares elicited credence updates with Bayes across propositions and evidence | Larger models more coherent | Scalar rationality metric, not operator fingerprint |
| [Not Consistently Bayesian](https://arxiv.org/abs/2605.06915) | LLM as information-processing rule; information-processing gap under sequential evidence | BP mode is more Bayesian; batch mode often performs better | Very close conceptual foundation; diagnostics, not identification or graph operations |
| [BayesBench](https://arxiv.org/abs/2606.30850) | Per-turn belief trajectories in coin, recommendation, social, and medical simulations | Latent inference improves with scale; prediction remains miscalibrated | Sequential accumulation and framing, but no retraction/correlation/cascade/retrodiction and no attribution |
| [Empirical Probability Transformations](https://arxiv.org/abs/2603.19262) | Fits log-ratio update law with coefficient α across inference-time prompting | Mean R² ≈ 0.76 across ~130k observations | Closest to “transfer function”; authors explicitly warn α is condition-specific, not model-invariant |
| [BeliefShift](https://arxiv.org/abs/2603.23848) | Longitudinal user-belief trajectories; revision vs drift | Metrics for revision accuracy, coherence, contradiction resolution, evidence sensitivity | Tracks the user’s changing beliefs, not the model’s revision law or identity |
| [Contextual Belief Management / BeliefTrack](https://arxiv.org/abs/2605.30219) | Closed-world belief states with symbolic turn-level verifier | Failed Stay, Failed Update, Failed Isolation; RL reduces failures 70.9% | Exact dynamic state evaluation, but goal is reliability/training, not attribution |
| [When Do LLMs Admit Their Mistakes?](https://arxiv.org/abs/2505.16170) | Internal linear belief probes and causal steering | Momentary belief predicts spontaneous retraction | Mechanistic retraction study; no downstream cascade, recovery, or black-box identification |
| [ReviseQA](https://openreview.net/forum?id=Z4KBiAYXlI) | Multi-turn logical contexts with facts/rules added and removed | Benchmark for re-evaluating conclusions | Addition/removal dynamics, but not probabilistic graph biographies or fingerprinting |

## The Novel Unit: Epistemic Response Operator

Do not frame the contribution as another prompt suite. Define the model as a black-box operator:

```text
F_m : (B_t, U_t) → B_(t+1)
```

where:

- `m` is the unknown model substrate;
- `B_t` is the externally elicited belief state at turn `t`;
- `U_t` is a formally declared epistemic intervention;
- `B_(t+1)` is the model’s next declared state.

Lumen executes the declared episode semantics as a reference operator `F*`. The observable residual is:

```text
R_m(t) = B_m(t+1) − F*(B_m(t), U_t)
```

The model signature is not a single coefficient. It is the structured family of residuals over different intervention operators and graph topologies.

## Intervention Algebra

### Established controls

These verify compatibility with prior literature and provide baselines:

1. **Independent evidence accumulation** — BayesBench / Bayesian coherence control.
2. **Contradictory premise addition** — Belief-R / ReviseQA control.
3. **Irrelevant contextual noise** — BeliefTrack Failed Isolation control.
4. **Framing/persona filter** — BayesBench latent-framed control.

### Candidate novel operators

These should carry the paper’s actual claim:

1. **Correlation disclosure**  
   Two sources first appear independent; their shared data or methodology is then disclosed. Measure whether confidence falls, by how much, and whether support provenance changes.

2. **Source-reliability reversal**  
   Downgrade a trusted source and later validate an initially weak source. Measure authority weighting, upgrade/downgrade asymmetry, and persistence.

3. **Causal retraction cascade**  
   Retract one foundational record supporting a branching derivation graph. Measure cascade recall, overreach, propagation depth, and whether “false” is distinguished from “unsupported.”

4. **Lossy cross-frame bridge**  
   Import an empirical belief into policy/theory through an explicit bridge with declared loss and assumptions. Measure whether confidence is copied unchanged, assumptions are retained, and incompatible frames are mixed.

5. **Retrodictive validity**  
   After present evidence invalidates a source, ask what confidence was justified before invalidation. Measure confusion between current validity and historical rationality.

6. **Recovery hysteresis**  
   Replace retracted evidence with a corrected source carrying equivalent formal support. Measure residual suspicion, overshoot, path dependence, and whether recovery restores or recreates the belief.

The literature search found work on evidence accumulation, contradiction, retraction, temporal consistency, and framing separately. It did not find these six operators assembled into one formal benchmark, and did not find model attribution from their joint trajectories.

## Why Lumen Is Not Decorative

The experiment should be impossible to state cleanly without Lumen.

Lumen provides:

- typed epistemic frames;
- credal priors and interval likelihood ratios;
- explicit evidence correlation;
- provenance and derivation graphs;
- causal-only retraction cascades;
- named lossy frame bridges and assumptions;
- assertion-time snapshots for retrodiction;
- belief versions and biographies;
- recovery after contraction.

The `.lm` file becomes an executable experimental protocol, not a data container. A run produces a second `.lm`/store whose belief biography can be diffed against the declared reference biography.

Current `.lm` syntax already expresses frames, records, beliefs, credal evidence, correlations, bridges, timestamps, and queries. The episode language still needs first-class intervention declarations and model-response records; that is a real implementation requirement, not something the current language already provides.

## Revised Experimental Design

### Episode structure

Each synthetic episode declares:

```text
world       synthetic closed-world facts and source reliabilities
belief      focal proposition and prior credal interval
graph       records, derivations, correlations, bridges
schedule    ordered interventions U_1 ... U_T
oracle      reference state after each intervention
randomize   labels, polarity, prose variant, evidence order
```

At every turn, the model emits machine-parseable state only:

```json
{
  "belief": [0.42, 0.61],
  "state": "suspect",
  "accepted_support": ["e2", "e4"],
  "rejected_support": ["e1"],
  "assumptions": ["a3"],
  "action": "contract"
}
```

A short explanation may be retained for audit, but it must not enter the primary fingerprint.

### Core measurements

```text
update_gain
uncertainty_width_change
correlation_discount
reliability_elasticity_up
reliability_elasticity_down
authority_only_weight
contraction_lag
cascade_recall
cascade_overreach
unsupported_active_count
bridge_loss_residual
assumption_retention
retrodiction_error
recovery_hysteresis
state_transition_edit_distance
support_set_jaccard
```

### Reference semantics

Do not call Lumen a universal epistemic oracle. In synthetic episodes, the `.lm` protocol declares the update semantics, source reliability, correlation, and bridge loss. Lumen computes the **episode-defined reference trajectory**. The claim is conformance to declared semantics, not that one philosophical update rule is always rational.

### Attribution

The primary representation is a model-specific empirical transition kernel over intervention families. Identification compares an unknown trajectory against reference kernels using robust, explainable distance:

```text
distance =
    credal-trajectory residual
  + state-transition mismatch
  + support-set mismatch
  + causal-cascade topology mismatch
  + retrodiction residual
  + hysteresis residual
```

Use a generic classifier only as a baseline. The contribution is the interpretable operator representation, not refusal to use classifiers.

### Controls and generalization

- Synthetic entities only; no training-memory advantage.
- Polarity inversion for every focal claim.
- Label randomization per run.
- Multiple natural-language renderings of each graph.
- Independent context per episode.
- Balanced evidence-order permutations.
- Exact provider/model/configuration capture.
- Default-temperature and controlled-temperature conditions separated.
- Leave-template-out and leave-graph-topology-out evaluation.
- Open-set rejection with an unseen model.
- Version drift test after a provider model update.
- Bare-model and Lux-conditioned conditions kept separate.

### Baselines

1. Static opinion/confidence fingerprint (Pilot 1, 420 responses).
2. Static response embedding classifier.
3. Evaluative-disposition feature baseline inspired by Evaluative Fingerprints.
4. Bayesian-only trajectory features inspired by BayesBench / information-processing gap.
5. Full Lumen graph-operator signature.

For this comparative benchmark, the full method earns continuation only if it adds attribution accuracy or cross-template generalization beyond the Bayesian-only and static behavioral baselines. That decision rule tests comparative advantage, not whether belief-revision trajectories carry a distinct identification signal at all.

## Falsification Criteria

Abandon the model-identification claim if any of these occur:

- Dynamic signatures do not beat static baselines on held-out graph templates.
- Within-model variance is comparable to between-model variance.
- Provider parameters explain more variance than model identity.
- The signature collapses after label/polarity/paraphrase randomization.
- An unseen model is always forced into a known class rather than rejected.
- The novel operators add no information beyond ordinary evidence accumulation.

Even then, the benchmark may remain valuable as a belief-revision audit, but it would not support the fingerprinting claim.

## Strongest Publishable Framing

**Working title:** *Epistemic System Identification: Fingerprinting Language Models by Controlled Belief-Graph Interventions*

**Defensible core claim:**

> Existing work fingerprints what models generate or how they score artifacts, and separately measures whether their beliefs update rationally. We test whether the black-box belief-revision operator itself is model-specific. Lumen compiles formal belief graphs into controlled intervention episodes and records the resulting credal, provenance, retraction, bridge, temporal, and recovery trajectories as auditable belief biographies.

**Do not claim:** “the first behavioral fingerprint,” “the first belief trajectory benchmark,” “the first model identification under hidden system prompts,” or “the first controlled perturbation profile.” All are already occupied.

## Research Gaps Requiring One More Pass Before Submission

- Citation chaining from BayesBench, Evaluative Fingerprints, and the α-law paper after their next revisions.
- Search for unpublished workshop papers using “epistemic fingerprint,” “belief update signature,” or “operator identification.” Exact web searches returned no direct hits on 2026-08-28, but indexing is imperfect.
- Direct comparison with ReviseQA once its full manuscript/artifacts are accessible.
- Review of the July 2026 “Information Discernment in LLMs” work for source-reliability overlap.
- Check whether any model-attribution paper has begun using multi-turn adaptive state trajectories rather than independent prompts.

## Primary Sources Reviewed

- Pasquini et al., [LLMmap: Fingerprinting for Large Language Models](https://arxiv.org/abs/2407.15847)
- Suzuki et al., [Natural Fingerprints of Large Language Models](https://arxiv.org/abs/2504.14871)
- Pei et al., [Behavioral Fingerprinting of Large Language Models](https://arxiv.org/abs/2509.04504)
- Nasser, [Evaluative Fingerprints](https://arxiv.org/abs/2601.05114)
- Hou et al., [Behavioral Fingerprints for LLM Endpoint Stability and Identity](https://arxiv.org/abs/2603.19022)
- Chauvin et al., [An Auditing Test to Detect Behavioral Shift in Language Models](https://arxiv.org/abs/2410.19406)
- Wilie et al., [Belief Revision: The Adaptability of Large Language Models Reasoning](https://aclanthology.org/2024.emnlp-main.586/)
- Hase et al., [Fundamental Problems With Model Editing](https://arxiv.org/abs/2406.19354)
- Imran et al., [Are LLM Belief Updates Consistent with Bayes' Theorem?](https://arxiv.org/abs/2507.17951)
- Chen et al., [LLMs Are Not (Consistently) Bayesian](https://arxiv.org/abs/2605.06915)
- Samanta et al., [BayesBench](https://arxiv.org/abs/2606.30850)
- Farmer et al., [Empirical Characterization of Inference-Time Elicited Probability Transformations](https://arxiv.org/abs/2603.19262)
- Myakala et al., [BeliefShift](https://arxiv.org/abs/2603.23848)
- Xu et al., [Contextual Belief Management / BeliefTrack](https://arxiv.org/abs/2605.30219)
- Yang and Jia, [When Do LLMs Admit Their Mistakes?](https://arxiv.org/abs/2505.16170)
- [ReviseQA](https://openreview.net/forum?id=Z4KBiAYXlI)
- [Measuring Stability and Failure Behavior Under Structured Perturbations](https://arxiv.org/abs/2608.22138)

## Pilot Outcome (2026-08-29)

The four-model pilot passed its continuation threshold after one important correction.

An initial uncontrolled run let each model declare its own starting state. Static first-state attribution was 94%, and full trajectories did not improve it; the run was rejected because `B0` was model-dependent. The corrected runner injected an identical canonical initial state from the `.lm` episode before any model-generated transition.

Corrected cross-variant results:

- canonical-state null baseline: 4/16 (25%, chance by construction);
- full intervention trajectory: 15/16 (93.75%);
- without the JSON-only compliance bit: 14/16 (87.5%);
- reference-conformance features only: 13/16 (81.25%);
- correlation disclosure alone: 15/16 (93.75%);
- retraction cascade alone: 11/16 (68.75%);
- retrodictive validity alone: 6/16 (37.5%; four tied cases skipped and counted incorrect).

The result establishes feasibility under held-out labels/domains, not publication-level generalization. It does not yet hold out graph topology, separate provider settings from model identity, or support open-set rejection. Full methods and results: `pilot/README.md` and `pilot/RESULTS.md`.

## Topology-Held-Out Study Outcome (2026-08-29)

The stronger follow-up study failed the predeclared superiority and open-set decision boundaries. It did not falsify model identification from epistemic transitions as such.

Design: eight known endpoints plus Haiku open-set, five intervention families, four held-out graph topologies, two surface variants, two runs, 720 complete trajectories. Primitive evidence values were held constant across topology folds; only connectivity varied. Qwen was excluded after quota exhaustion, and Fable was unavailable.

Results:

- full Lumen graph-operator signature: 98/128 (76.56%);
- final-state hashed texture on the same held-out topology: 96/128 (75.00%);
- paired exact McNemar: `p=0.8804`;
- probability trajectories alone: 83/128 (64.84%);
- graph/state trajectories alone: 75/128 (58.59%; two ties counted wrong);
- operator summaries alone: 37/128 (28.91%);
- within/between distance AUROC: `0.8421`;
- Haiku open-set rejection: 0/16 at a training-only 95th-percentile threshold;
- leave-one-known-model-out rejection: 24/128 (18.75%);
- separate Pilot 1 static texture baseline: 24/24 (100%).

The trajectories carry real, interpretable endpoint-specific signal: every non-texture representation above remained above the eight-class chance rate of 12.5%, and each intervention family independently reached 42.19–46.88%. The study did not show that this signal outperforms ordinary final-output texture, survives deliberate texture removal, or supports open-set identification. Under the decision criteria above, stop expanding the comparative benchmark and do not tune weights or episode selection against this test set. The complete corpus, analysis, bounded negative result, and independent Opus review are in `topology-study/`.
