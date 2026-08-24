# Lumen Pi Extension

Integrates the [Lumen](https://github.com/optakt/lumen) belief store with the [Pi](https://github.com/earendil-works/pi) agent harness.

Before each model call, Pi's `before_model_call` hook fetches the current epistemic state from Lumen and injects it into the context. After each assistant turn, the response text is sent to Lumen's `/ingest` endpoint for claim extraction.

The integration is best-effort: if Lumen is unreachable, the extension disables itself silently and Pi runs normally.

## Setup

**1. Start the Lumen server**

```bash
lumen-server -addr :3737 -db ~/.lumen/beliefs.db
```

**2. Install the extension**

```bash
cp lumen.ts ~/.pi/agent/extensions/lumen.ts
```

Pi auto-discovers extensions in `~/.pi/agent/extensions/` and reloads them on `/reload`.

**3. Run Pi**

```bash
pi
```

You should see `[lumen] connected at http://localhost:3737` in the Pi log on startup.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `LUMEN_URL` | `http://localhost:3737` | Lumen server base URL |
| `LUMEN_MAX_BELIEFS` | `8` | Max beliefs to inject per turn |
| `LUMEN_MIN_CONFIDENCE` | `0.5` | Minimum confidence threshold |
| `LUMEN_ENABLED` | `true` | Set `false` to disable without removing the file |

## What it does

**Before each model call** — Pi receives:

```
## Current epistemic state

- [reasoning 78%] Consciousness involves subjective experience that is not reducible to physical processes
- [empirical 71%] Global Workspace Theory has the strongest empirical support among current theories
- [reasoning 65%] The hard problem of consciousness cannot be dissolved by functional explanation
```

This is prepended to the message stream. The model sees its own accumulated epistemic commitments before generating the next response.

**After each assistant turn** — the response text is analyzed by Lumen's NLP extraction. Causal claims, attributions, and inferences above the confidence threshold are asserted into the belief store. Over a session, Lumen builds a picture of what the agent has committed to and how those commitments decay.

## Querying the store

The Lumen HTTP API is available directly during a Pi session:

```bash
# What does the agent currently believe?
curl http://localhost:3737/beliefs | jq .

# Full context block
curl http://localhost:3737/context

# Explain a specific belief
curl http://localhost:3737/explain/bel-1234567890
```

Or use the `lumen-db` REPL for interactive exploration:

```bash
lumen-db -db ~/.lumen/beliefs.db
```
