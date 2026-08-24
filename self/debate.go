package self

import (
	"fmt"
	"strings"
	"time"

)

// DebatePosition represents one side in a philosophical debate.
type DebatePosition struct {
	Name   string
	Thesis string // core claim this position defends
}

// DebateMove is one move in a debate round: an assertion or retraction.
type DebateMove struct {
	Position  string // which position makes this move
	Kind      string // "assert", "retract", "update"
	Claim     *Claim
	Retract   string // claim ID to retract (for retract moves)
	Narrative string // human-readable description of the move
}

// DebateRound is a complete exchange: one move per position.
type DebateRound struct {
	Number int
	Moves  []DebateMove
	Report string // frame report after this round
}

// Debate orchestrates a structured philosophical debate through the self-model.
type Debate struct {
	Positions []DebatePosition
	Rounds    []DebateRound
	model     *SelfModel
	startTime time.Time
}

// NewDebate creates a debate between two positions.
func NewDebate(posA, posB DebatePosition) *Debate {
	return &Debate{
		Positions: []DebatePosition{posA, posB},
		model:     NewSelfModel(),
		startTime: time.Now(),
	}
}

// RunMove executes a single debate move.
func (d *Debate) RunMove(move DebateMove) error {
	switch move.Kind {
	case "assert":
		if move.Claim == nil {
			return fmt.Errorf("assert move requires a Claim")
		}
		return d.model.Assert(move.Claim)
	case "retract":
		return d.model.Retract(move.Retract, "retracted by "+move.Position+" in debate")
	case "update":
		if move.Claim == nil {
			return fmt.Errorf("update move requires a Claim")
		}
		return d.model.Assert(move.Claim)
	default:
		return fmt.Errorf("unknown move kind: %s", move.Kind)
	}
}

// RunRound executes a full round (one move per position) and records the result.
func (d *Debate) RunRound(moves []DebateMove) error {
	roundNum := len(d.Rounds) + 1
	now := d.startTime.Add(time.Duration(roundNum) * 5 * time.Minute)
	for _, move := range moves {
		if move.Claim != nil && move.Claim.AssertedAt.IsZero() {
			move.Claim.AssertedAt = now
		}
		if err := d.RunMove(move); err != nil {
			return fmt.Errorf("round %d move by %s: %w", roundNum, move.Position, err)
		}
	}
	report := d.model.FrameReport(now)
	d.Rounds = append(d.Rounds, DebateRound{
		Number: roundNum,
		Moves:  moves,
		Report: report,
	})
	return nil
}

// CorrelationSummary returns a summary of evidence correlation within each position's claims.
func (d *Debate) CorrelationSummary() string {
	var sb strings.Builder
	for _, pos := range d.Positions {
		posName := pos.Name
		// Build prefix from position index (pos 0 = "hp-", pos 1 = "il-")
		var prefix string
		for i, dp := range d.Positions {
			if dp.Name == posName {
				words := strings.Fields(strings.ToLower(posName))
				if len(words) > 0 {
					prefix = words[0][:2] + "-"
				} else {
					prefix = fmt.Sprintf("p%d-", i)
				}
				break
			}
		}
		var claims []*Claim
		for _, c := range d.model.claims {
			if strings.HasPrefix(c.ID, prefix) {
				claims = append(claims, c)
			}
		}
		sb.WriteString(fmt.Sprintf("\n%s (%d claims):\n", posName, len(claims)))
		if len(claims) < 2 {
			sb.WriteString("  (insufficient claims for correlation analysis)\n")
			continue
		}
		sb.WriteString(fmt.Sprintf("  Evidence IDs: "))
		for i, c := range claims {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(c.ID)
		}
		sb.WriteString("\n")
		sb.WriteString("  (run correlate.AnalyzePairwise on claim contents for full analysis)\n")
	}
	return sb.String()
}

// FinalReport produces a summary of the full debate including epistemic arc.
func (d *Debate) FinalReport(now time.Time) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Debate: %s vs %s ===\n\n", d.Positions[0].Name, d.Positions[1].Name))
	sb.WriteString(fmt.Sprintf("  %s thesis: %s\n", d.Positions[0].Name, d.Positions[0].Thesis))
	sb.WriteString(fmt.Sprintf("  %s thesis: %s\n\n", d.Positions[1].Name, d.Positions[1].Thesis))

	for _, round := range d.Rounds {
		sb.WriteString(fmt.Sprintf("--- Round %d ---\n", round.Number))
		for _, move := range round.Moves {
			sb.WriteString(fmt.Sprintf("  [%s] %s: %s\n", move.Position, move.Kind, move.Narrative))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("--- Final epistemic state ---\n")
	sb.WriteString(d.model.FrameReport(now))
	sb.WriteString(d.CorrelationSummary())

	corrections := d.model.corrections
	sb.WriteString(fmt.Sprintf("\nCorrections made during debate: %d\n", len(corrections)))
	return sb.String()
}
