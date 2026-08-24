# The `.lm` File Format

`.lm` (Lumen Model) is the native format for Lumen belief stores. A `.lm` file declares frames, records, beliefs, bridges, and named queries as a structured document. Files are plain text, indentation-delimited, and designed to be both human-readable and machine-parseable.

## Syntax Overview

`.lm` files use keyword-introduced blocks delimited by indentation. Top-level declarations start at column 0; their properties are indented one level (two spaces). Comments begin with `//`.

```lumen
// A minimal .lm file
frame reasoning
  decay: none

record r1 in reasoning
  "The zombie argument: a world physically identical to ours but with no qualia is conceivable."
  at: "2024-01-15"

believe b1 in reasoning
  "The hard problem of consciousness is genuine."
  confidence: 0.78
  from: r1
```

---

## Declarations

### `frame`

Defines an epistemic frame — a context within which records and beliefs are asserted. Frames carry decay policies and composition rules.

```
frame <name>
  decay: <policy>
  [composition: <mode>]
  [on_stale_derivation: <action>]
  [calibration: "<method>"]
```

**Properties:**

| Property | Values | Description |
|---|---|---|
| `decay` | See [Decay Policies](#decay-policies) | How confidence fades over time |
| `composition` | `bayesian` (default), `opaque` | Whether Bayesian evidence decomposition is allowed |
| `on_stale_derivation` | `mark_suspect`, `fail`, `retry` | Behaviour when a derived belief's sources fall below threshold |
| `calibration` | string | Hint for the calibrate tool on how to measure confidence |

**Example:**

```lumen
frame empirical
  decay: exponential halflife: 730d
  composition: bayesian
  on_stale_derivation: mark_suspect

frame parametric
  decay: step at: 365d to: 0.5
  composition: opaque
  calibration: "frequentist count"
```

---

### `record`

An immutable observation or piece of evidence. Records are the leaves of the provenance tree — they cannot themselves be derived.

```
record <id> in <frame>
  "<content>"
  [at: "<ISO-8601 date or datetime>"]
  [provenance: foundational]
```

**Properties:**

| Property | Description |
|---|---|
| `content` | The observation text (quoted string) |
| `at` | When the observation was made; defaults to load time |
| `provenance: foundational` | Marks this record as a chain terminus — WeakestLink analysis stops here |

**Example:**

```lumen
record cogitate-2023 in empirical
  "Cogitate Consortium: prefrontal cortex activity not necessary for conscious experience. N=256, multi-lab replication."
  at: "2023-04-18"
  provenance: foundational
```

---

### `believe`

A belief: a claim with a confidence level derived from records and other beliefs.

```
believe <id> in <frame>
  "<content>"
  confidence: <0.0–1.0>
  [from: <id> [<id> ...]]
  [prior: <0.0–1.0>]
  [evidence]
    <evidence blocks>
```

**Properties:**

| Property | Description |
|---|---|
| `content` | The claim text (quoted string) |
| `confidence` | Declared confidence at assertion time |
| `from` | Source IDs (records or beliefs) this belief derives from |
| `prior` | Prior probability before evidence (for Bayesian composition) |
| `evidence` | Inline evidence blocks for credal Bayesian composition (see below) |

**Simple derivation:**

```lumen
record r-workspace-broadcast in reasoning
  "Conscious content is broadcast widely across cortical regions."

believe b-gwt in reasoning
  "Global Workspace Theory has the strongest empirical support."
  confidence: 0.72
  from: r-workspace-broadcast
```

**With Bayesian composition:**

```lumen
believe b-hardproblem in reasoning
  "The hard problem of consciousness is genuine and resists functional reduction."
  confidence: 0.78
  prior: 0.5
  evidence r-zombie
    lr: 3.2
    confidence: 0.80
  evidence r-knowledge
    lr: 2.1
    confidence: 0.70
```

The evidence block ID names the source record or belief. A point prior
(`prior: 0.5`) is the degenerate interval `[0.5, 0.5]`.

**With credal (interval) priors and interval likelihood ratios:**

```lumen
believe b-panpsychism in reasoning
  "Panpsychism is a live option."
  confidence: 0.45
  prior: [0.3, 0.6]
  evidence r-combination-problem
    lr: [0.5, 0.8]
    confidence: 0.65
```

**Correlated evidence** shares underlying support; declare the correlation
so composition avoids double-counting:

```lumen
  evidence r-zombie
    lr: 3.2
    confidence: 0.80
    correlates-with: r-conceivability 0.61
```

---

### `bridge`

A named translation between two frames, recording loss, method, and assumptions.

```
bridge <name> : <from-frame> → <to-frame>
  loss: <0.0–1.0>
  method: "<description>"
  verified: <true|false>
  [assumes: "<assumption>"]
```

**Example:**

```lumen
bridge empirical-to-reasoning : empirical → reasoning
  loss: 0.15
  method: "informal inference from experimental result to philosophical position" verified: false
  assumes: "experimental design validity"
```

---

### `query`

A named event query over the belief history.

```
query <name>
  [where: <predicate>]
  [since: "<ISO-8601>"]
  [until: "<ISO-8601>"]
```

**Predicates:**

| Predicate | Example | Matches |
|---|---|---|
| `confidence < N` | `confidence < 0.5` | Beliefs below threshold |
| `confidence > N` | `confidence > 0.8` | Beliefs above threshold |
| `frame = "<name>"` | `frame = "empirical"` | Beliefs in a specific frame |
| `state = "<state>"` | `state = "suspect"` | Beliefs in a given state |
| `content contains "<text>"` | `content contains "consciousness"` | Content substring match |
| `AND`, `OR`, `NOT` | `confidence > 0.5 AND frame = "empirical"` | Boolean combinators |

**Example:**

```lumen
query high-confidence-empirical
  where: confidence > 0.7 AND frame = "empirical" since: "2024-01-01"
```

---

## Decay Policies

Decay policies control how a belief's confidence changes over time.

| Policy | Syntax | Behaviour |
|---|---|---|
| `none` | `decay: none` | Confidence is constant |
| `exponential` | `decay: exponential halflife: <duration>` | Confidence halves every `halflife` |
| `linear` | `decay: linear rate: <rate>` | Confidence decreases by `rate` per day |
| `step` | `decay: step at: <duration> to: <value>` | Confidence drops to `to` once `at` has elapsed since assertion |

**Duration formats:** `30d`, `6m`, `1y`, `48h`

---

## Composition Modes

| Mode | Description |
|---|---|
| `bayesian` | Standard Bayesian composition using likelihood ratios. FragilityScan uses exact sensitivity analysis. |
| `opaque` | Evidence decomposition disabled. The confidence value is taken as-is; calibrate cannot improve it. Use for externally-calibrated estimates (e.g. prediction markets, frequentist counts). |

---

## Stale Derivation Actions

When a belief's sources (its `from:` list) decay below the `StaleThreshold`:

| Action | Description |
|---|---|
| `mark_suspect` | Mark the belief suspect — still visible, flagged with ⚠ in context output |
| `fail` | Return an error from operations that read this belief |
| `retry` | Mark suspect and propagate the flag through dependent beliefs |

---

## Import Resolution

`.lm` files can import definitions from other files:

```lumen
import "./foundations.lm"
import "./empirical-base.lm"
```

Imports are resolved at load time. Circular imports are detected and rejected.

---

## Complete Example

```lumen
// consciousness.lm — epistemic model of consciousness philosophy

frame parametric
  decay: none

frame empirical
  decay: exponential halflife: 730d
  on_stale_derivation: mark_suspect

frame reasoning
  decay: exponential halflife: 1825d

record r-cogitate in empirical
  "Cogitate Consortium found prefrontal cortex is not necessary for conscious experience. N=256, multi-lab replication."
  at: "2023-04-18"
  provenance: foundational

record r-zombie in reasoning
  "The zombie argument: a physically identical world with no qualia is conceivable, and conceivability implies metaphysical possibility."
  at: "2024-01-01"

record r-knowledge in reasoning
  "Mary's Room: a scientist who knows all physical facts about colour vision learns something new upon first seeing red."
  at: "2024-01-01"

believe b-gwt in reasoning
  "Global Workspace Theory has the strongest empirical support among current consciousness theories."
  confidence: 0.72
  from: r-cogitate

believe b-hardproblem in reasoning
  "The hard problem of consciousness is genuine and resists functional reduction."
  confidence: 0.78
  prior: 0.5
  evidence r-zombie
    lr: 3.2
    confidence: 0.80
  evidence r-knowledge
    lr: 2.1
    confidence: 0.70

bridge reasoning-to-parametric : reasoning → parametric
  loss: 0.20
  method: "philosophical argument to stable background commitment"
  verified: false

query recent-high-confidence
  target: b-hardproblem
  select: confidence-changes
  since: "2024-01-01"
```

---

## Grammar Summary (EBNF)

```ebnf
file         ::= (declaration | import | comment)*
declaration  ::= frame-decl | record-decl | believe-decl | bridge-decl | query-decl
import       ::= "import" quoted-string
comment      ::= "//" text newline

frame-decl   ::= "frame" name newline frame-body
frame-body   ::= indent "decay:" decay-policy newline
                       ["composition:" comp-mode newline]
                       ["on_stale_derivation:" stale-action newline]
                       ["calibration:" quoted-string newline]
                 dedent

record-decl  ::= "record" id "in" name newline record-body
record-body  ::= indent quoted-string newline
                       ["at:" quoted-string newline]
                       ["provenance:" "foundational" newline]
                 dedent

believe-decl ::= "believe" id "in" name newline believe-body
believe-body ::= indent quoted-string newline
                       "confidence:" float newline
                       ["from:" id+ newline]
                       ["prior:" (float | "[" float "," float "]") newline]
                       [evidence-block]
                 dedent

evidence-block ::= "evidence" id newline
                   indent
                     "lr:" (float | "[" float "," float "]") newline
                     ["confidence:" float newline]
                     ["source:" id newline]
                     ["correlates-with:" id float newline]
                   dedent

bridge-decl  ::= "bridge" id ":" name "→" name newline bridge-body
bridge-body  ::= indent "loss:" float newline
                        "method:" quoted-string newline
                        "verified:" bool newline
                        ["assumes:" quoted-string newline]
                 dedent

query-decl   ::= "query" id newline query-body
query-body   ::= indent ["where:" predicate newline]
                        ["since:" quoted-string newline]
                        ["until:" quoted-string newline]
                 dedent

decay-policy ::= "none"
               | "exponential" "halflife:" duration
               | "linear" "rate:" float
               | "step" "at:" duration "to:" float

comp-mode    ::= "bayesian" | "opaque"
stale-action ::= "mark_suspect" | "fail" | "retry"
duration     ::= number ("d" | "h" | "m" | "y")
```
