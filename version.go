package lumen

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// RecordVersion captures a record immediately before a revision or retraction.
type RecordVersion struct {
	Record       Record
	ChangedAt    time.Time
	ChangeReason string
}

func (s *Store) snapshotRecord(record *Record, changedAt time.Time, reason string) {
	copyRecord := cloneRecord(record)
	s.recordVersions[record.ID] = append(s.recordVersions[record.ID], RecordVersion{
		Record: *copyRecord, ChangedAt: changedAt, ChangeReason: reason,
	})
}

// BeliefVersion records the state of a belief at a specific point in time.
// Versions are created automatically when a belief is materially changed
// via ReAssert or Revise.
type BeliefVersion struct {
	Version             int
	Content             string
	Confidence          float64
	Frame               string
	State               BeliefState
	Derivation          []string // source IDs at the time of snapshot; nil = unchanged from previous
	ContractedBy        string
	ImportedDecay       []DecayPolicy
	DecayOverride       *DecayPolicy
	CrossFrame          []CrossFrameSource
	CompositionPrior    float64
	CompositionEvidence []Evidence
	AssertedAt          time.Time
	ChangedAt           time.Time
	ChangeReason        string
}

// VersionStore maintains the version history for all beliefs.
type VersionStore struct {
	mu       sync.RWMutex
	versions map[string][]BeliefVersion // beliefID → ordered versions (oldest first)
}

func NewVersionStore() *VersionStore {
	return &VersionStore{versions: make(map[string][]BeliefVersion)}
}

// Snapshot records the current state of a belief as a version.
// Called before any material change to the belief.
func (vs *VersionStore) Snapshot(b *Belief, changedAt time.Time, reason string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	history := vs.versions[b.ID]
	version := len(history) + 1
	copyBelief := cloneBelief(b)
	vs.versions[b.ID] = append(vs.versions[b.ID], BeliefVersion{
		Version:             version,
		Content:             b.Content,
		Confidence:          b.Confidence,
		Frame:               b.Frame,
		State:               b.State,
		Derivation:          copyBelief.Derivation,
		ContractedBy:        copyBelief.ContractedBy,
		ImportedDecay:       copyBelief.ImportedDecay,
		DecayOverride:       copyBelief.DecayOverride,
		CrossFrame:          copyBelief.CrossFrame,
		CompositionPrior:    copyBelief.CompositionPrior,
		CompositionEvidence: copyBelief.CompositionEvidence,
		AssertedAt:          b.AssertedAt,
		ChangedAt:           changedAt,
		ChangeReason:        reason,
	})
}

// History returns all versions of a belief, oldest first.
func (vs *VersionStore) History(beliefID string) []BeliefVersion {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	h := vs.versions[beliefID]
	result := make([]BeliefVersion, len(h))
	for i := range h {
		result[i] = cloneBeliefVersion(h[i])
	}
	return result
}

// VersionAt returns the state of a belief at or before the given time.
// Returns nil if no version existed at that time.
func (vs *VersionStore) VersionAt(beliefID string, t time.Time) *BeliefVersion {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	history := vs.versions[beliefID]
	var best *BeliefVersion
	for i := range history {
		v := history[i]
		if !v.AssertedAt.After(t) {
			copyV := cloneBeliefVersion(v)
			best = &copyV
		}
	}
	return best
}

func cloneBeliefVersion(v BeliefVersion) BeliefVersion {
	v.Derivation = append([]string(nil), v.Derivation...)
	v.ImportedDecay = append([]DecayPolicy(nil), v.ImportedDecay...)
	v.CrossFrame = append([]CrossFrameSource(nil), v.CrossFrame...)
	v.CompositionEvidence = append([]Evidence(nil), v.CompositionEvidence...)
	if v.DecayOverride != nil {
		copyDecay := *v.DecayOverride
		v.DecayOverride = &copyDecay
	}
	return v
}

func cloneRecordVersions(history []RecordVersion) []RecordVersion {
	result := make([]RecordVersion, len(history))
	for i := range history {
		result[i] = history[i]
		copyRecord := cloneRecord(&history[i].Record)
		result[i].Record = *copyRecord
	}
	return result
}

// Diff returns a human-readable description of what changed between two versions.
func Diff(a, b BeliefVersion) string {
	var changes []string
	if a.Content != b.Content {
		changes = append(changes, fmt.Sprintf("content: %q → %q", truncateDiff(a.Content), truncateDiff(b.Content)))
	}
	if a.Frame != b.Frame {
		changes = append(changes, fmt.Sprintf("frame: %s → %s", a.Frame, b.Frame))
	}
	if fmt.Sprintf("%.3f", a.Confidence) != fmt.Sprintf("%.3f", b.Confidence) {
		changes = append(changes, fmt.Sprintf("confidence: %.0f%% → %.0f%%", a.Confidence*100, b.Confidence*100))
	}
	if a.State != b.State {
		changes = append(changes, fmt.Sprintf("state: %s → %s", stateToString(a.State), stateToString(b.State)))
	}
	if len(changes) == 0 {
		return "(no material changes)"
	}
	return strings.Join(changes, "; ")
}

func truncateDiff(s string) string {
	if len(s) > 50 {
		return s[:47] + "..."
	}
	return s
}

// BeliefHistory returns the full version history of a belief.
func (s *Store) BeliefHistory(beliefID string) ([]BeliefVersion, error) {
	s.mu.RLock()
	if _, ok := s.beliefs[beliefID]; !ok {
		s.mu.RUnlock()
		return nil, fmt.Errorf("belief %s not found", beliefID)
	}
	s.mu.RUnlock()
	return s.versions.History(beliefID), nil
}

// RenderHistory returns a human-readable version history.
func RenderHistory(beliefID string, versions []BeliefVersion) string {
	if len(versions) == 0 {
		return fmt.Sprintf("No version history for %s (first version is the current state).\n", beliefID)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Version history for %s (%d snapshots):\n\n", beliefID, len(versions))
	for i, v := range versions {
		fmt.Fprintf(&b, "  v%d  %s", v.Version, v.ChangedAt.Format("2006-01-02 15:04"))
		if v.ChangeReason != "" {
			fmt.Fprintf(&b, "  [%s]", v.ChangeReason)
		}
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "       %.0f%%  %s  [%s]\n", v.Confidence*100, v.Content, v.Frame)
		if i < len(versions)-1 {
			d := Diff(v, versions[i+1])
			fmt.Fprintf(&b, "       → %s\n", d)
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}
