# Lumen Hermes Integration

Integrates the [Lumen](https://github.com/optakt/lumen) belief store with the [Hermes](https://github.com/NousResearch/hermes-agent) agent harness via its shell hook system.

Two hooks wire into the agent loop:

- **`pre_llm_call`** — injects the current epistemic state as context before each LLM call
- **`post_llm_call`** — extracts claims from the assistant response and asserts them into the store

The integration is a single Go binary (`lumen-hook`) that reads Hermes's JSON payload from stdin and calls the Lumen HTTP API. It fails open: if Lumen is unreachable, the hook exits cleanly and Hermes continues normally.

## Setup

**1. Build the hook binary**

```bash
go install github.com/optakt/lumen/cmd/lumen-hook@latest
go install github.com/optakt/lumen/cmd/lumen-server@latest
```

**2. Start the Lumen server**

```bash
lumen-server -addr :3737 -db ~/.lumen/beliefs.db
```

**3. Configure Hermes**

Add to `~/.hermes/cli-config.yaml`:

```yaml
hooks:
  pre_llm_call:
    - command: "lumen-hook"
  post_llm_call:
    - command: "lumen-hook"
```

Hermes will prompt for shell hook consent on first use. Accept both entries.

**4. Run Hermes**

```bash
hermes
```

On the first turn after a session builds up some claims, you'll see the belief block prepended to each LLM call.

## Wire protocol

Hermes pipes a JSON payload to `lumen-hook` stdin for each event:

```json
{
  "hook_event_name": "pre_llm_call",
  "session_id": "sess_abc123",
  "extra": {}
}
```

For `pre_llm_call`, the hook outputs `{"context": "..."}` and Hermes injects the text. For `post_llm_call`, the hook reads `extra.assistant_response` and POSTs it to `/ingest`. No stdout on `post_llm_call` — Hermes doesn't use a response directive for that event.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `LUMEN_URL` | `http://localhost:3737` | Lumen server base URL |
| `LUMEN_MAX_BELIEFS` | `8` | Max beliefs to inject per turn |
| `LUMEN_MIN_CONFIDENCE` | `0.5` | Minimum confidence threshold |

## What it looks like

After a few turns, Hermes receives something like this before each LLM call:

```
## Current epistemic state

- [reasoning 85%] The retrodiction problem is real in belief revision systems
- [empirical 74%] Tryptophan networks show UV superradiance at room temperature (Babcock et al. 2024)
- [reasoning 68%] Global Workspace Theory has the strongest empirical support of current theories
```

Claims made in earlier turns that have decayed below the confidence threshold drop out automatically.

## Querying the store

```bash
# What does the agent currently believe?
curl http://localhost:3737/beliefs | jq .

# Full context block
curl http://localhost:3737/context

# Explain a specific belief
curl http://localhost:3737/explain/bel-1234567890

# Interactive REPL
lumen-db -db ~/.lumen/beliefs.db
```
