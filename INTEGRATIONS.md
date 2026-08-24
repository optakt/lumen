# Integrations

Lumen exposes a REST API over HTTP, letting any agent framework track beliefs without being coupled to Go or the Lumen codebase. The server handles persistence; the agent framework handles when to call it.

## Pi

[Pi](https://github.com/earendil-works/pi) (earendil-works/pi) is a TypeScript agent harness with a lifecycle extension system. The Lumen extension hooks two events: `before_model_call` to inject the current epistemic state, and `after_model_call` to extract and assert new claims.

See [`integrations/pi/`](integrations/pi/) for the extension file and full setup guide.

**Quick start:**

```bash
# 1. Build and start the Lumen server
go install github.com/optakt/lumen/cmd/lumen-server@latest
lumen-server -addr :3737 -db ~/.lumen/beliefs.db

# 2. Install the Pi extension
cp integrations/pi/lumen.ts ~/.pi/agent/extensions/lumen.ts

# 3. Run Pi
pi
# → [lumen] connected at http://localhost:3737
```

**What a session looks like:**

After a few turns, before each model call Pi receives something like:

```
## Current epistemic state

- [reasoning 82%] The hard problem of consciousness is not dissoluble by functional explanation
- [empirical 74%] Tryptophan networks show UV superradiance at room temperature (Babcock et al. 2024)
- [reasoning 68%] Global Workspace Theory has the strongest empirical support among current theories
- [reasoning 61%] Retrodiction — applying decay policies retroactively — is an error in belief revision systems
```

The model sees its own accumulated epistemic commitments. Claims made in earlier turns that have decayed below the confidence threshold drop out of the injected context automatically.

**Querying the store during a session:**

```bash
# List all active beliefs
curl http://localhost:3737/beliefs | jq .

# Get the current context block
curl http://localhost:3737/context

# Explain why a specific belief has its current confidence
curl http://localhost:3737/explain/bel-1234567890

# Full REPL
lumen-db -db ~/.lumen/beliefs.db
```

## HTTP API Reference

The `lumen-server` binary exposes the following endpoints:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Liveness check |
| `GET` | `/context` | Compact belief block for context injection |
| `GET` | `/beliefs` | List all active beliefs with current confidence |
| `POST` | `/records` | Assert an immutable empirical record |
| `POST` | `/believe` | Assert a derived belief with confidence |
| `POST` | `/retract` | Retract a record (cascades suspect marking to derived beliefs) |
| `POST` | `/ingest` | Extract claims from raw text and assert them |
| `GET` | `/explain/{id}` | Natural language explanation of a belief's current state |

### `GET /context`

Query params:

| Param | Default | Description |
|-------|---------|-------------|
| `max` | `10` | Max beliefs to include |
| `min_confidence` | `0.5` | Filter threshold |
| `format` | `text` | `text` or `json` |

Returns a ranked list of active beliefs, sorted by current confidence. The text format is designed to drop directly into a context message.

### `POST /ingest`

```json
{
  "text": "The study found that meditation reduces cortisol. Therefore mindfulness appears to have genuine physiological effects.",
  "frame": "empirical",
  "min_confidence": 0.6
}
```

Runs NLP extraction over `text`, automatically detecting frames and linking derived beliefs to their source records. Returns counts of what was asserted and skipped.

### `POST /believe`

```json
{
  "content": "The hard problem of consciousness is not dissoluble by functional analysis",
  "confidence": 0.82,
  "frame": "reasoning",
  "sources": ["rec-abc123"]
}
```

### `lumen-server` flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:3737` | Listen address |
| `-db` | `lumen.db` | BoltDB database path |
| `-max-beliefs` | `10` | Max beliefs in `/context` |
| `-min-confidence` | `0.5` | Min confidence for `/context` |
| `-ingest-confidence` | `0.55` | Min confidence for `/ingest` |
| `-frame` | `reasoning` | Default frame for ingested beliefs |

## Hermes

[Hermes](https://github.com/NousResearch/hermes-agent) uses a shell hook system where any executable can register for agent lifecycle events. The `lumen-hook` binary reads Hermes's JSON payload from stdin and calls the Lumen HTTP API.

See [`integrations/hermes/`](integrations/hermes/) for the setup guide.

**Quick start:**

```bash
# 1. Build the binaries
go install github.com/optakt/lumen/cmd/lumen-hook@latest
go install github.com/optakt/lumen/cmd/lumen-server@latest

# 2. Start the Lumen server
lumen-server -addr :3737 -db ~/.lumen/beliefs.db

# 3. Add to ~/.hermes/cli-config.yaml
```

```yaml
hooks:
  pre_llm_call:
    - command: "lumen-hook"
  post_llm_call:
    - command: "lumen-hook"
```

Hermes prompts for shell hook consent on first use.

## Other frameworks

The HTTP API is framework-agnostic. Any harness that can make HTTP calls or spawn subprocesses can integrate with Lumen using the two-hook pattern:

1. Before model call: `GET /context` → inject text into context
2. After assistant turn: `POST /ingest` → extract and persist claims

For shell-hook systems (Hermes, Cursor): use the `lumen-hook` binary.
For programmatic hooks (Pi, Mastra): call the HTTP API directly.
