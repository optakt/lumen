# Lumen OpenClaw Plugin

Integrates [Lumen](https://github.com/optakt/lumen) with [OpenClaw](https://github.com/openclaw/openclaw).

Two hooks wire into the agent loop:

- **`agent_turn_prepare`** — fetches the current epistemic state and prepends it to context before each model call. Both general beliefs and self-model commitments are injected.
- **`llm_output`** — extracts claims from the assistant's response via Lumen's NLP pipeline and asserts them into the belief store.

The plugin degrades gracefully: if Lumen is unreachable, both hooks skip silently.

## Setup

**1. Build and start the Lumen server**

```bash
go install github.com/optakt/lumen/cmd/lumen-server@latest
lumen-server -addr :3737 -db ~/.lumen/beliefs.db
```

**2. Install the plugin**

Copy the plugin directory into your OpenClaw plugins folder, or install from source:

```bash
cp -r integrations/openclaw ~/.openclaw/plugins/lumen
```

Or add directly to `openclaw.config.json`:

```json
{
  "plugins": {
    "lumen": {
      "url": "http://localhost:3737",
      "maxBeliefs": 8,
      "minConfidence": 0.5,
      "selfContext": true
    }
  }
}
```

**3. Run OpenClaw**

```bash
openclaw
```

## Configuration

| Key | Default | Description |
|---|---|---|
| `url` | `http://localhost:3737` | Lumen server base URL |
| `maxBeliefs` | `8` | Max beliefs to inject per turn |
| `minConfidence` | `0.5` | Confidence threshold for injection and extraction |
| `selfContext` | `true` | Include self-model commitments in injected context |

## What gets injected

Before each model call, OpenClaw prepends:

```
## Current epistemic state

- [reasoning 85%] The retrodiction problem arises when decay policies are applied retroactively to imported beliefs
- [empirical 74%] Tryptophan networks show UV superradiance at room temperature (Babcock et al. 2024)
- [reasoning 68%] Global Workspace Theory has the strongest empirical support among current theories

## My current epistemic commitments

- [reasoning 87%] Decay policies should apply from assertion time, not import time
- [reasoning 74% ⚠] Illusionism fails to explain why there is something it is like to be in pain
```

Claims from earlier turns that decay below the confidence threshold drop out automatically. The ⚠ marker indicates a belief that has been retracted (superseded by a correction) but is still visible so the model knows its prior commitment.

## Querying the store

```bash
curl http://localhost:3737/beliefs | jq .
curl http://localhost:3737/context
curl http://localhost:3737/self/context
curl http://localhost:3737/explain/bel-1234567890
```
