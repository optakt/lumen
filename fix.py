import re

# Fix 1: replace expApprox with math.Exp in belief.go
with open('belief.go', 'r') as f:
    content = f.read()

# Add math import and replace expApprox usage
content = content.replace(
    'import (\n\t"time"\n)',
    'import (\n\t"math"\n\t"time"\n)'
)

# Replace the pow2neg function to use math.Exp
old_pow2neg = '''func pow2neg(x float64) float64 {
	// 2^(-x) via natural log: e^(-x * ln2)
	const ln2 = 0.693147180559945
	return expApprox(-x * ln2)
}

// Simple Taylor series for e^x, accurate enough for our purposes
func expApprox(x float64) float64 {
	if x < -20 {
		return 0
	}
	result := 1.0
	term := 1.0
	for i := 1; i <= 20; i++ {
		term *= x / float64(i)
		result += term
		if term < 1e-10 && term > -1e-10 {
			break
		}
	}
	if result < 0 {
		return 0
	}
	return result
}'''

new_pow2neg = '''func pow2neg(x float64) float64 {
	return math.Exp(-x * math.Ln2)
}'''

content = content.replace(old_pow2neg, new_pow2neg)

with open('belief.go', 'w') as f:
    f.write(content)

print("Fixed belief.go")

# Fix 2: in store.go, also register beliefs as dependents of records
with open('store.go', 'r') as f:
    sc = f.read()

old = '''	for _, srcID := range b.Derivation {
		if rec, ok := s.records[srcID]; ok {
			if rec.Retracted {
				b.State = BeliefSuspect
			}
			// Records don't carry decay; no import needed
			_ = rec
		} else if src, ok := s.beliefs[srcID]; ok {'''

new = '''	for _, srcID := range b.Derivation {
		if rec, ok := s.records[srcID]; ok {
			if rec.Retracted {
				b.State = BeliefSuspect
			}
			// Register dependency on record too, so retraction propagates
			if s.dependents[srcID] == nil {
				s.dependents[srcID] = make(map[string]bool)
			}
			s.dependents[srcID][b.ID] = true
		} else if src, ok := s.beliefs[srcID]; ok {'''

sc = sc.replace(old, new)

with open('store.go', 'w') as f:
    f.write(sc)

print("Fixed store.go")
