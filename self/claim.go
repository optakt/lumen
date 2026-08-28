package self

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lumen "github.com/optakt/lumen"
)

var sentinelSequence atomic.Uint64

// ClaimKind identifies what kind of epistemic act produced a belief.
type ClaimKind string

const (
	// Asserted: "I believe X" — the agent is claiming something directly.
	ClaimAsserted ClaimKind = "asserted"
	// Derived: "Because A and B, I conclude X" — derivation from other beliefs.
	ClaimDerived ClaimKind = "derived"
	// Retrieved: "I recall that X" — from memory or archive.
	ClaimRetrieved ClaimKind = "retrieved"
	// Corrected: "I was wrong; X" — replaces a prior belief.
	ClaimCorrected ClaimKind = "corrected"
)

// Claim is a structured assertion the agent makes during a conversation.
// It wraps a Lumen belief with metadata about what kind of epistemic act produced it.
type Claim struct {
	ID         string
	Kind       ClaimKind
	Content    string // what is being claimed
	Frame      string // which epistemic frame
	Confidence float64
	AssertedAt time.Time
	Derivation []string // IDs of beliefs/records this follows from
	Replaces   string   // ID of a prior belief this corrects (if ClaimCorrected)
	Tags       []string // topic tags for later retrieval
}

// SelfModel is the agent's live epistemic state — a Lumen belief store
// representing what the agent believes, how it knows it, and how confident it is.
type SelfModel struct {
	mu           sync.Mutex // guards claims and corrections; store locks itself
	store        *lumen.Store
	claims       map[string]*Claim // claim ID -> Claim
	corrections  []Correction      // history of corrections
	sessionStart time.Time
}

// Correction records a moment where a prior claim was retracted or revised.
type Correction struct {
	At          time.Time
	RetractedID string
	Reason      string
	ReplacedBy  string // ID of new claim, if any
}

func NewSelfModel() *SelfModel {
	s := lumen.NewStore()
	RegisterSelfFrames(s)
	return &SelfModel{
		store:        s,
		claims:       make(map[string]*Claim),
		sessionStart: time.Now(),
	}
}

// Assert records a claim the agent is making.
//
// When the claim corrects a prior one (Replaces set), the new claim is
// asserted first and the prior retracted only after success — the reverse
// order loses the old claim without gaining the new one if assertion fails.
func (m *SelfModel) Assert(c *Claim) error {
	if c == nil {
		return fmt.Errorf("claim must not be nil")
	}
	copyClaim := *c
	copyClaim.Derivation = append([]string(nil), c.Derivation...)
	copyClaim.Tags = append([]string(nil), c.Tags...)
	c = &copyClaim
	if c.AssertedAt.IsZero() {
		c.AssertedAt = time.Now()
	}
	if c.ID == "" {
		return fmt.Errorf("claim ID must not be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.claims[c.ID]; exists {
		return fmt.Errorf("claim %s already exists", c.ID)
	}
	if c.Replaces != "" {
		if _, exists := m.claims[c.Replaces]; !exists {
			return fmt.Errorf("claim %s to replace not found", c.Replaces)
		}
		if _, err := m.sentinelForClaim(c.Replaces); err != nil {
			return err
		}
	}
	if err := m.store.ValidateBeliefCandidate(&lumen.Belief{
		ID: c.ID, Content: c.Content, Confidence: c.Confidence,
		AssertedAt: c.AssertedAt, Frame: c.Frame, Derivation: c.Derivation,
	}); err != nil {
		return fmt.Errorf("believe: %w", err)
	}

	// Create a validity sentinel record for this claim.
	// Retracting the sentinel will mark the belief as suspect.
	sentinelID := fmt.Sprintf("sentinel:%s-%d", c.ID, sentinelSequence.Add(1))
	if err := m.store.Assert(&lumen.Record{
		ID:        sentinelID,
		Content:   "validity sentinel for " + c.ID,
		Timestamp: c.AssertedAt,
		Frame:     c.Frame,
	}); err != nil {
		return fmt.Errorf("sentinel: %w", err)
	}

	derivation := append([]string{sentinelID}, c.Derivation...)
	belief := &lumen.Belief{
		ID:         c.ID,
		Content:    c.Content,
		Confidence: c.Confidence,
		AssertedAt: c.AssertedAt,
		Frame:      c.Frame,
		Derivation: derivation,
	}
	if err := m.store.Believe(belief); err != nil {
		// Clean up the sentinel so a retried assertion doesn't conflict.
		_ = m.store.Retract(sentinelID, "orphaned: claim assertion failed", c.AssertedAt)
		return fmt.Errorf("believe: %w", err)
	}

	// The new claim is in place — now retract the one it corrects.
	if c.Replaces != "" {
		if err := m.retractLocked(c.Replaces, "corrected: superseded by "+c.ID, c.AssertedAt); err != nil {
			return fmt.Errorf("new claim %s asserted, but retracting prior %s failed: %w", c.ID, c.Replaces, err)
		}
		m.corrections = append(m.corrections, Correction{
			At: c.AssertedAt, RetractedID: c.Replaces,
			Reason: "corrected", ReplacedBy: c.ID,
		})
	}

	m.claims[c.ID] = c
	return nil
}

// Retract marks a prior claim as invalid (e.g. corrected by new information).
// It does NOT assert a replacement — use Assert with Replaces for that.
func (m *SelfModel) Retract(claimID string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.retractLocked(claimID, reason, time.Now())
}

func (m *SelfModel) retractLocked(claimID, reason string, at time.Time) error {
	sentinelID, err := m.sentinelForClaim(claimID)
	if err != nil {
		return err
	}
	return m.store.Retract(sentinelID, reason, at)
}

func (m *SelfModel) sentinelForClaim(claimID string) (string, error) {
	for _, sourceID := range m.store.Graph.DerivationSources(claimID) {
		if strings.HasPrefix(sourceID, "sentinel:") {
			return sourceID, nil
		}
	}
	legacy := "sentinel:" + claimID
	if m.store.HasRecord(legacy) {
		return legacy, nil
	}
	return "", fmt.Errorf("validity sentinel for claim %s not found", claimID)
}

// Status returns the current confidence of a claim, with decay applied.
func (m *SelfModel) Status(claimID string, now time.Time) (*lumen.QueryResult, error) {
	return m.store.Query(claimID, now)
}

// Reflect generates a meta-level analysis of a claim's epistemic status.
func (m *SelfModel) Reflect(claimID string, now time.Time) (*lumen.ReflectiveAnswer, error) {
	return m.store.Reflect(lumen.ReflectiveQuery{
		TargetBeliefID: claimID,
		Question:       "is_well_calibrated",
	}, now)
}

// FrameReport returns a summary of what the agent believes in each frame.
func (m *SelfModel) FrameReport(now time.Time) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var sb strings.Builder
	sb.WriteString("Self-model report\n")
	sb.WriteString(fmt.Sprintf("Session started: %s\n", m.sessionStart.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Corrections made: %d\n", len(m.corrections)))
	sb.WriteString("\nBeliefs by frame:\n")

	// Group claims by frame
	byFrame := make(map[string][]*Claim)
	for _, c := range m.claims {
		byFrame[c.Frame] = append(byFrame[c.Frame], c)
	}

	for _, frameName := range []string{"parametric", "retrieved", "reasoning", "reflective"} {
		claims := byFrame[frameName]
		if len(claims) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n  [%s]\n", frameName))
		for _, c := range claims {
			result, err := m.Status(c.ID, now)
			if err != nil {
				continue
			}
			status := ""
			if result.State == lumen.BeliefSuspect {
				status = " [RETRACTED]"
			}
			sb.WriteString(fmt.Sprintf("    %.0f%% %q%s\n",
				result.CurrentConfidence*100, c.Content, status))
		}
	}

	if len(m.corrections) > 0 {
		sb.WriteString("\nCorrection history:\n")
		for _, corr := range m.corrections {
			sb.WriteString(fmt.Sprintf("  %s: retracted %q (%s)",
				corr.At.Format("15:04:05"), corr.RetractedID, corr.Reason))
			if corr.ReplacedBy != "" {
				sb.WriteString(fmt.Sprintf(" → replaced by %q", corr.ReplacedBy))
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// EpistemicStatus returns a human-readable description of a claim's epistemic quality.
func (m *SelfModel) EpistemicStatus(claimID string, now time.Time) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.claims[claimID]
	if !ok {
		return fmt.Sprintf("claim %s not found", claimID)
	}

	result, err := m.Status(claimID, now)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Claim: %q\n", c.Content))
	sb.WriteString(fmt.Sprintf("Frame: %s | Kind: %s\n", c.Frame, c.Kind))
	sb.WriteString(fmt.Sprintf("Confidence: %.0f%%", result.CurrentConfidence*100))
	if result.State == lumen.BeliefSuspect {
		sb.WriteString(" [RETRACTED — do not use]")
	}
	sb.WriteString("\n")

	// Frame-specific epistemic warnings
	switch c.Frame {
	case "parametric":
		sb.WriteString("⚠ Parametric: provenance inaccessible, training cutoff unknown, cannot verify.\n")
	case "retrieved":
		sb.WriteString("📚 Retrieved: from memory or archive. May be summarized. Cross-frame loss possible.\n")
	case "reasoning":
		sb.WriteString("💭 Reasoning: derived within this session. Fresh but not independently verified.\n")
	case "reflective":
		sb.WriteString("🔍 Reflective: a claim about my own epistemic state. Adversarially activated.\n")
	}

	if len(c.Derivation) > 0 {
		sb.WriteString(fmt.Sprintf("Derived from: %s\n", strings.Join(c.Derivation, ", ")))
	}
	return sb.String()
}
