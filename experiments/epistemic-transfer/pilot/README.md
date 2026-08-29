# Epistemic System Identification Pilot

This pilot tests whether black-box LLMs can be identified from how they transform a formally declared belief state under controlled graph interventions.

It is deliberately small and falsifiable:

- 4 models: Claude Opus 4.6, GPT-5.6 Sol, Grok 4.6, DeepSeek V4 Pro
- 3 intervention families:
  - correlation disclosure
  - causal retraction cascade
  - retrodictive validity
- 2 label/domain variants per family
- 2 independent runs per model/episode
- 48 complete trajectories, 128 model-generated transitions plus 48 canonical seed states

## Why the episodes are `.lm`

Each file in `episodes/` is an executable experimental protocol: synthetic world, focal claim, declared prior, intervention schedule, and episode-defined reference biography. The `transfer` package parses the protocol and compares model-declared states with its reference states.

The syntax is an experimental Lumen extension, intentionally kept outside the stable `LM_SPEC.md` until the pilot establishes that the mechanism carries signal.

## Run

```bash
./run.sh
```

The script resolves provider keys via `optakt secret`; raw keys never enter files or model context.

Resume is automatic. A completed `(model, episode, run)` trajectory is skipped.

## Analyze

```bash
go run ../../../cmd/epistemic-transfer \
  -mode analyze \
  -episodes episodes \
  -results results.jsonl
```

Evaluation is cross-variant:

- train on variant A, identify held-out variant B
- train on variant B, identify held-out variant A

Two representations are compared:

1. **Static baseline** — only the first declared state from each family
2. **Dynamic signature** — full trajectory, reference residuals, state/action transitions, support-set agreement, cascade topology, correlation discount, and retrodiction error

The initial state is injected by Lumen rather than elicited from the model. This keeps `B0` identical and prevents static confidence or formatting differences from masquerading as transition dynamics.

## Falsification rule

The pilot fails if the full dynamic signature does not beat the static first-state baseline on held-out variants, or if accuracy is near 25% chance. A failure still validates the runner as a belief-revision audit, but not model identification.
