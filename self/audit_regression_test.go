package self

import (
	"testing"
	"time"

	lumen "github.com/optakt/lumen"
)

func TestAssertRejectsNilClaim(t *testing.T) {
	if err := NewSelfModel().Assert(nil); err == nil {
		t.Fatal("Assert(nil) should return an error")
	}
}

func TestAssertCanRetryAfterMissingDerivation(t *testing.T) {
	model := NewSelfModel()
	now := time.Now()
	claim := &Claim{
		ID: "claim", Kind: ClaimDerived, Frame: "reasoning", Content: "derived",
		Confidence: 0.8, AssertedAt: now, Derivation: []string{"source"},
	}
	if err := model.Assert(claim); err == nil {
		t.Fatal("expected missing-source failure")
	}
	if err := model.store.Believe(&lumen.Belief{
		ID: "source", Frame: "reasoning", Content: "source", Confidence: 0.9, AssertedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := model.Assert(claim); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
}

func TestAssertCopiesClaimAndPreflightsReplacement(t *testing.T) {
	model := NewSelfModel()
	now := time.Now()
	claim := &Claim{ID: "original", Kind: ClaimAsserted, Frame: "reasoning", Content: "original", Confidence: 0.8, AssertedAt: now}
	if err := model.Assert(claim); err != nil {
		t.Fatal(err)
	}
	claim.Content = "mutated"
	if got := model.EpistemicStatus("original", now); got == "" || containsText(got, "mutated") {
		t.Fatalf("caller mutation leaked into self-model: %s", got)
	}

	correction := &Claim{
		ID: "replacement", Kind: ClaimCorrected, Frame: "reasoning", Content: "new",
		Confidence: 0.9, AssertedAt: now.Add(time.Hour), Replaces: "missing",
	}
	if err := model.Assert(correction); err == nil {
		t.Fatal("replacement of missing claim should fail preflight")
	}
	if _, err := model.Status("replacement", now.Add(time.Hour)); err == nil {
		t.Fatal("failed replacement was nevertheless asserted")
	}
}

func TestCorrelationSummaryHandlesShortPositionName(t *testing.T) {
	debate := NewDebate(DebatePosition{Name: "A"}, DebatePosition{Name: "B"})
	_ = debate.CorrelationSummary()
}

func containsText(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
