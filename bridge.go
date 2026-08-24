package lumen

import (
	"sort"
	"fmt"
	"strings"
	"sync"
)

// Bridge declares a named translation between two epistemic frames.
// A bridge is not a function — it's an explicit epistemological commitment
// about what is lost and what is preserved when evidence crosses frame boundaries.
//
// Bridges make cross-frame derivation auditable:
//   - The bridge is named, so it appears in provenance traces
//   - Loss annotations propagate — a belief that crossed a bridge carries the loss
//   - verified: false is honest — some bridges are engineering compromises
type Bridge struct {
	Name     string
	FromFrame string
	ToFrame   string
	// Loss describes what gets collapsed or discarded in the translation.
	// Format: "collapses(plausibility, belief)" or "discretizes(confidence)"
	Loss string
	// Method is the mathematical procedure for the translation (e.g. "pignistic")
	Method string
	// Verified indicates whether the bridge has a known-good mathematical foundation.
	// false is not an error — it's honest declaration.
	Verified bool
	// Assumptions is a human-readable description of what the bridge assumes.
	Assumptions string
}

// BridgeRegistry holds all declared bridges in a store.
type BridgeRegistry struct {
	mu      sync.RWMutex
	bridges map[string]*Bridge // name → bridge
	// Index by (fromFrame, toFrame) for quick lookup
	byFrames map[string][]*Bridge // "from→to" → bridges
}

func NewBridgeRegistry() *BridgeRegistry {
	return &BridgeRegistry{
		bridges:  make(map[string]*Bridge),
		byFrames: make(map[string][]*Bridge),
	}
}

// Register adds a bridge to the registry.
func (r *BridgeRegistry) Register(b *Bridge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.bridges[b.Name]; exists {
		return fmt.Errorf("bridge %s already registered", b.Name)
	}
	r.bridges[b.Name] = b
	key := b.FromFrame + "→" + b.ToFrame
	r.byFrames[key] = append(r.byFrames[key], b)
	return nil
}

// Lookup returns a bridge by name.
func (r *BridgeRegistry) Lookup(name string) (*Bridge, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.bridges[name]
	return b, ok
}

// BridgesFor returns all declared bridges from one frame to another.
func (r *BridgeRegistry) BridgesFor(fromFrame, toFrame string) []*Bridge {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := fromFrame + "→" + toFrame
	return r.byFrames[key]
}

// RequiresBridge returns true if the store should require an explicit bridge
// when a belief in toFrame derives from a source in fromFrame.
// All returns all registered bridges sorted by name.
func (r *BridgeRegistry) All() []*Bridge {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Bridge, 0, len(r.bridges))
	for _, b := range r.bridges { out = append(out, b) }
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *BridgeRegistry) RequiresBridge(fromFrame, toFrame string) bool {
	if fromFrame == toFrame {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := fromFrame + "→" + toFrame
	_, exists := r.byFrames[key]
	// If any bridges are declared for this pair, crossing without naming one
	// is an error. If no bridges are declared, the crossing is uncontrolled but silent.
	return exists
}

// BridgeCrossing records that a belief crossed a frame boundary via a specific bridge.
type BridgeCrossing struct {
	BridgeName string
	FromFrame  string
	ToFrame    string
	// LossCarried is the cumulative loss annotation from this bridge
	// and any bridges crossed upstream in the derivation chain.
	LossCarried string
}

// AccumulateLoss merges loss annotations from a new bridge crossing.
// Loss is additive: if you cross two bridges each collapsing different things,
// the downstream belief carries both loss annotations.
func AccumulateLoss(existing, newLoss string) string {
	if existing == "" {
		return newLoss
	}
	if newLoss == "" {
		return existing
	}
	// Deduplicate if same loss appears twice in a chain
	if strings.Contains(existing, newLoss) {
		return existing
	}
	return existing + "; " + newLoss
}

// BridgedBelief extends Belief with bridge crossing metadata.
// A belief knows which bridges were crossed in its derivation chain
// and what cumulative loss those crossings introduced.
type BridgedBelief struct {
	Belief
	// Crossings is the list of bridge crossings in derivation order.
	Crossings []BridgeCrossing
	// CumulativeLoss is the accumulated loss from all crossings.
	// Empty means the belief's derivation stays within a single frame.
	CumulativeLoss string
}

// IsTranslated returns true if this belief crossed at least one bridge.
func (b *BridgedBelief) IsTranslated() bool {
	return len(b.Crossings) > 0
}

// ProvenanceAnnotation returns a human-readable summary of the translation chain.
func (b *BridgedBelief) ProvenanceAnnotation() string {
	if !b.IsTranslated() {
		return ""
	}
	var parts []string
	for _, c := range b.Crossings {
		verified := "unverified"
		if _, ok := b.Crossings[0].LossCarried, false; ok {
			verified = "verified"
		}
		_ = verified
		parts = append(parts, fmt.Sprintf("%s (%s→%s)", c.BridgeName, c.FromFrame, c.ToFrame))
	}
	return fmt.Sprintf("translated via %s; accumulated loss: %s",
		strings.Join(parts, " → "), b.CumulativeLoss)
}
