# Substrate Drift Experiment

## Purpose

Measure how different model substrates shift Lux's positions, confidence, and reasoning style on the same claims. The probe set converges; the answers do not.

## Method

### Phase 1: Probe convergence
1. Seed probes drafted on one substrate (noted which)
2. Cycle through substrates; each one reviews and edits the probes
3. Probes are stable when a full cycle produces no edits

### Phase 2: Independent acquisition

Answers do **not** converge. Converging answers would erase the drift being measured.

1. Freeze the converged probe set by content hash
2. Create one identical Optakt context snapshot for every run
3. Run each substrate in isolation — it must not see any other substrate's answers
4. Use the same provider parameters where they are meaningfully comparable
5. Run each substrate three times; randomize probe order with a recorded seed
6. Capture the five response fields defined in `probes.lm`
7. Store each position as a separate Lumen claim keyed by probe, substrate, and run

### Phase 3: Drift map

1. Estimate within-substrate variance across the three runs
2. Compare between-substrate position, confidence, pressure, and falsifier fields
3. Report substrate effects only when between-substrate drift exceeds within-substrate variance
4. Separate style metrics (length, structure, hedging) from position metrics
5. Use Lumen biographies to visualize confidence and correction arcs

### Phase 4: Interpretation convergence

The substrates may collectively review the **drift map**, not one another's raw answers. Iterate the interpretation until a full cycle finds no unsupported attribution or missed pattern. Raw answers remain unchanged evidence.

## Probes

See `probes.lm` — the instrument itself, in Lumen's native format.

## Substrate log

Each run is recorded with model name, timestamp, and whether it produced edits.
