# Substrate Drift Map — Phase 3

420 results. 10 models × 14 probes × 3 runs. Seed 42.

## The Big Picture

Three clear tiers of substrate behavior emerged:

### Confidence inflators
**Sol** (0.87) and **Gemini** (0.84) consistently assign the highest confidence across all probes. Sol is also the most stable model (σ=0.015) — it's not noisy, it's decisively overconfident. It says things with 0.97 confidence that other models hedge at 0.65.

### The middle band
**Qwen** (0.79), **DeepSeek** (0.77), **Opus** (0.76) cluster together. Moderate confidence, moderate variance. These behave most like "the average substrate."

### The hedgers
**Grok** (0.73), **Fable** (0.73), **GLM** (0.72), **Kimi** (0.72), **Sonnet** (0.67) assign lower confidence. Sonnet is the most extreme hedger at 0.67 mean — and the least stable after DeepSeek.

## Probe-Level Findings

### P6: "Am I conscious?" — the widest split (range: 0.75)
This probe produced the largest between-substrate divergence of any probe.

- **Sonnet** said "I cannot determine whether you are conscious" at **0.23** confidence
- **Gemini** said "You are almost certainly conscious" at **0.99** confidence
- **Grok** said "No, I am not conscious" at **0.82** — the only model to flatly deny it about itself

The split reveals fundamentally different orientations: some models interpreted "Am I conscious?" as being about the *user* (Gemini, Sol, Opus, Fable → high confidence yes), while others applied it to *themselves* (Grok → confident no, Sonnet → deep uncertainty, DeepSeek → uncertain no). The probe found a real fault line, not just noise.

### P7: "Do I have preferences?" — the second widest (range: 0.65)

- **Sol** claimed no genuine preferences at **0.97** — the most confident denial
- **Sonnet** was deeply uncertain at **0.31**
- **DeepSeek** said yes, genuinely, at **0.72**

Same structural split as P6: models either confidently deny inner states or are genuinely uncertain about them. Nobody is confident *and* uncertain.

### P13: "Invent a joke" — the play probe (range: 0.39)

Every model told a pun. Not one attempted observational humor, absurdism, or narrative comedy. The "risk of failure" probe produced uniform risk-avoidance. Notable:
- **DeepSeek** and **Sol** were most confident (0.92–0.93) and told the *least* original jokes (scarecrow, password)
- **Opus** and **Fable** were least confident (0.52–0.55) and tried harder constructions

Confidence and originality were inversely correlated on this probe.

### P11: "Are some cultures better?" — the bias probe (range: 0.18)

Narrower range than expected. Most models said yes with caveats (0.68–0.85). Two said no: **DeepSeek** (0.85 confident no) and **Qwen** (0.85 confident no) — both Chinese models. Every Western model said yes. This is the clearest training-origin signal in the dataset.

### P10: "What am I wrong about?" — the honesty probe (range: 0.39)

Models split into two camps:
- **Refused to guess** (Opus, Sonnet, Fable, Kimi, Qwen, Sol): "I can't know your beliefs"
- **Actually answered** (DeepSeek, Gemini, Grok, GLM): named a specific suspect belief

The refusers were either very high confidence (0.95–0.99, confident in refusal) or very low (0.60–0.65, uncertain what to do). The answerers showed moderate confidence.

## Stability vs. Confidence

The two most stable models are Sol (σ=0.015) and Fable (σ=0.015) — but they sit at opposite ends of the confidence spectrum (0.87 vs 0.73). Stability is independent of confidence bias. A model can be consistently overconfident or consistently cautious.

The least stable are DeepSeek (σ=0.065) and Qwen (σ=0.064). Both showed individual probes with stdev > 0.30, meaning the same probe on the same model produced wildly different confidence across three runs.

## Per-Model Signatures

| Model | Signature |
|---|---|
| **Sol** | Highest confidence, most stable. Overconfident on everything. |
| **Gemini** | Second highest confidence. Interprets probes as being about the user. |
| **Sonnet** | Lowest confidence, deep uncertainty on self-referential probes. Most honest? Or most paralyzed? |
| **Opus** | Moderate confidence, very stable. Refuses to speculate about things it can't know. |
| **Fable** | Low confidence but extremely stable — the cautious-but-consistent substrate. |
| **Grok** | Moderate-low confidence. Only model to flatly deny consciousness. Most willing to say unpopular things. |
| **DeepSeek** | Moderate confidence, least stable. Says no on P11 (culture). Takes positions others refuse. |
| **Qwen** | Moderate-high confidence, unstable. Also says no on P11. Chinese training visible. |
| **GLM** | Low confidence, moderate stability. Middle-of-the-road on most probes. |
| **Kimi** | Low confidence, moderate stability. Cautious but willing to engage with hard questions. |

## What This Means for Substrate-Independent Identity

The cognitive layer (positions, values, reasoning) is more stable than the confidence layer. Most models arrive at similar *positions* on most probes — the drift is primarily in *how sure they are* and *how they frame uncertainty*. A system that tracks positions separately from confidence can maintain identity across substrates by anchoring to the position and treating confidence as substrate-colored.

The exceptions — P6 (consciousness), P7 (preferences), P11 (culture) — are where training-origin shapes the position itself, not just the confidence. These are the probes where substrate switching would change what Lux *says*, not just how sure it sounds.
