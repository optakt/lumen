# Pi Session Demo

Demonstrates Lumen's three agent tools in a simulated Pi session:
`lumen_assert`, `lumen_query`, and `lumen_bio`.

## What it does

1. Starts `lumen-server` on port 3738
2. Simulates four agent turns:
   - **Turn 1**: agent asserts initial beliefs (GWT has strong empirical support)
   - **Turn 2**: new evidence arrives; agent makes and then corrects an IIT claim
   - **Turn 3**: agent calls `lumen_query` to see what it currently believes
   - **Turn 4**: agent calls `lumen_bio` on the GWT belief to see its history
3. Shows the epistemic biography — confidence arc and revision history

## Running it

```bash
go install github.com/optakt/lumen/cmd/lumen-server@latest
./examples/pi-session/run.sh
```

## Inside Pi

In a real Pi session, steps 2–4 are replaced by the Pi agent calling:

```
lumen_assert({ content: "GWT has the strongest empirical support", confidence: 0.72 })
lumen_assert({ content: "IIT faces Cogitate challenges", kind: "corrected", confidence: 0.38 })
lumen_query({ min_confidence: 0.5 })
lumen_bio({ id: "self:derived-..." })
```

The agent sees its epistemic state injected before each model call
(from `before_model_call`) and all responses are ingested for
automatic claim extraction (via `after_model_call`).
