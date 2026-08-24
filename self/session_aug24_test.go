package self

import (
	"testing"
	"time"
)

// TestSession_Aug24 models the epistemic state of the August 24 Lumen development session.
// Claims span parametric knowledge, retrieved facts from the codebase, and reasoning.
// This is a real self-model of today's session: adaptive thinking mode, first use.
func TestSession_Aug24(t *testing.T) {
	m := NewSelfModel()
	now := time.Date(2026, 8, 24, 13, 0, 0, 0, time.UTC)

	// --- PARAMETRIC CLAIMS (from training) ---

	m.Assert(&Claim{
		ID:         "agm-postulates",
		Kind:       ClaimAsserted,
		Content:    "The AGM postulates (Alchourrón, Gärdenfors, Makinson 1985) are the standard framework for rational belief revision: closure, success, inclusion, vacuity, preservation, extensionality, conjunctive inclusion, conjunctive overflow",
		Frame:      "parametric",
		Confidence: 0.92,
		AssertedAt: now,
		Tags:       []string{"epistemology", "formal"},
	})

	m.Assert(&Claim{
		ID:         "interval-probability-advantage",
		Kind:       ClaimAsserted,
		Content:    "Imprecise probability (credal sets) gives more honest uncertainty quantification than point estimates: posterior intervals are honest about what the evidence cannot determine",
		Frame:      "parametric",
		Confidence: 0.85,
		AssertedAt: now,
		Tags:       []string{"epistemology", "probability"},
	})

	m.Assert(&Claim{
		ID:         "go-rwmutex-reader-accumulation",
		Kind:       ClaimAsserted,
		Content:    "Go's sync.RWMutex: when a writer is waiting, new RLock calls block — preventing reader accumulation starvation. Same goroutine calling RLock twice while a writer waits can deadlock.",
		Frame:      "parametric",
		Confidence: 0.88,
		AssertedAt: now,
		Tags:       []string{"concurrency", "go"},
	})

	m.Assert(&Claim{
		ID:         "boltdb-json-marshal",
		Kind:       ClaimAsserted,
		Content:    "BoltDB uses []byte storage; json.Marshal/Unmarshal on a Go struct automatically includes all public fields. Adding a public field to a struct makes it persist without other changes.",
		Frame:      "parametric",
		Confidence: 0.95,
		AssertedAt: now,
		Tags:       []string{"database", "go"},
	})

	// --- RETRIEVED CLAIMS (from reading codebase this session) ---

	m.Assert(&Claim{
		ID:         "lexer-line-by-line",
		Kind:       ClaimRetrieved,
		Content:    "Lumen lexer processes one line at a time (Tokenize splits on \\n, calls lexLine per line). Quoted strings must start and end on the same line — multi-line strings cause unterminated string errors.",
		Frame:      "retrieved",
		Confidence: 1.00,
		AssertedAt: now,
		Tags:       []string{"parser", "lexer"},
	})

	m.Assert(&Claim{
		ID:         "removenode-stale-index",
		Kind:       ClaimRetrieved,
		Content:    "BeliefGraph.RemoveNode filtered g.edges but did not rebuild g.outbound/g.inbound. Old indices pointed to wrong positions after the slice shrank, causing index-out-of-range panics in AddEdge during Recover().",
		Frame:      "retrieved",
		Confidence: 1.00,
		AssertedAt: now,
		Tags:       []string{"bug", "graph"},
	})

	m.Assert(&Claim{
		ID:         "soft-delete-preserves-recovery",
		Kind:       ClaimRetrieved,
		Content:    "ApplyContraction previously called delete(s.beliefs, bID) after tombstoning. This made K÷5 recovery impossible — the belief was gone. Soft-delete (keep in s.beliefs with BeliefSuperseded + ContractedBy) preserves recovery.",
		Frame:      "retrieved",
		Confidence: 1.00,
		AssertedAt: now,
		Tags:       []string{"agm", "recovery"},
	})

	m.Assert(&Claim{
		ID:         "parser-no-default-case",
		Kind:       ClaimRetrieved,
		Content:    "The frame parser switch had no default case for unknown attributes. After consuming key and colon, the VALUE token became the next KEY in the next iteration, causing parse failures on attributes like on_stale_derivation.",
		Frame:      "retrieved",
		Confidence: 1.00,
		AssertedAt: now,
		Tags:       []string{"parser", "bug"},
	})

	// --- REASONING CLAIMS (derived this session) ---

	m.Assert(&Claim{
		ID:         "retry-suspect-then-error",
		Kind:       ClaimDerived,
		Content:    "on_stale_derivation: retry should mark BeliefSuspect AND return error. Without marking suspect, every query re-fires the error; with marking suspect, subsequent queries show degraded state without repeated errors.",
		Frame:      "reasoning",
		Confidence: 0.88,
		AssertedAt: now,
		Tags:       []string{"design", "staleness"},
	})

	m.Assert(&Claim{
		ID:         "export-lm-slash-comments",
		Kind:       ClaimDerived,
		Content:    "ExportLM used # comments but the lexer only strips // comments. Exported files could not be re-parsed. The fix is to use // in the export header.",
		Frame:      "reasoning",
		Confidence: 1.00,
		AssertedAt: now,
		Tags:       []string{"export", "parser"},
	})

	m.Assert(&Claim{
		ID:         "superseded-filter-everywhere",
		Kind:       ClaimDerived,
		Content:    "After soft-delete, BeliefSuperseded beliefs appear in AllBeliefs. QueryBeliefs, BuildSearchIndex, list, calibrate, and ConflictScan all need explicit filters. AllBeliefs intentionally returns everything for full audit.",
		Frame:      "reasoning",
		Confidence: 0.92,
		AssertedAt: now,
		Tags:       []string{"design", "soft-delete"},
	})

	// Session reports
	t.Log("\n" + m.FrameReport(now))

	// Epistemic status on key design claims
	t.Log(m.EpistemicStatus("removenode-stale-index", now))
	t.Log(m.EpistemicStatus("soft-delete-preserves-recovery", now))
	t.Log(m.EpistemicStatus("retry-suspect-then-error", now))

	// After 1 week — retrieved context decays, parametric stays sharper
	week := now.Add(7 * 24 * time.Hour)
	t.Log("\n--- After 1 week ---\n" + m.FrameReport(week))
}
