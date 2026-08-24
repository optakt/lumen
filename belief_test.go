package lumen

import (
	"fmt"
	"testing"
	"time"
)

var medicalFrame = Frame{
	Name:        "medical_diagnosis",
	Composition: "bayesian",
	Decay: DecayPolicy{
		Kind:     "exponential",
		Halflife: 30 * 24 * time.Hour, // 30 days
	},
	ProvenanceDepth:     3,
	ImportedDecayPolicy: "most_conservative",
	OnStaleDerivation:   "mark_suspect",
}

var sensorFrame = Frame{
	Name:        "sensor_fusion",
	Composition: "bayesian",
	Decay: DecayPolicy{
		Kind:     "exponential",
		Halflife: 1 * time.Hour, // sensor readings decay fast
	},
	ProvenanceDepth:     2,
	ImportedDecayPolicy: "most_conservative",
}

func TestDecayExponential(t *testing.T) {
	policy := DecayPolicy{Kind: "exponential", Halflife: 30 * 24 * time.Hour}
	now := time.Now()

	cases := []struct {
		elapsed  time.Duration
		original float64
		wantMin  float64
		wantMax  float64
	}{
		{0, 0.9, 0.899, 0.901},                           // no time: unchanged
		{30 * 24 * time.Hour, 0.9, 0.449, 0.451},         // one halflife: ~0.45
		{60 * 24 * time.Hour, 0.9, 0.224, 0.226},         // two halflives: ~0.225
		{90 * 24 * time.Hour, 0.9, 0.112, 0.114},         // three halflives: ~0.1125
		{365 * 24 * time.Hour, 0.9, 0.0, 0.03},           // a year: nearly zero
	}

	for _, tc := range cases {
		got := policy.ApplyDecay(tc.original, tc.elapsed)
		if got < tc.wantMin || got > tc.wantMax {
			t.Errorf("elapsed=%v original=%.2f: got %.4f, want [%.3f, %.3f]",
				tc.elapsed, tc.original, got, tc.wantMin, tc.wantMax)
		}
	}
	_ = now
}

func TestDecayLinear(t *testing.T) {
	policy := DecayPolicy{Kind: "linear", Rate: 0.1} // loses 0.1 per day
	cases := []struct {
		elapsed time.Duration
		want    float64
	}{
		{0, 0.9},
		{1 * 24 * time.Hour, 0.8},
		{9 * 24 * time.Hour, 0.0}, // hits floor
		{15 * 24 * time.Hour, 0.0}, // stays at floor
	}
	for _, tc := range cases {
		got := policy.ApplyDecay(0.9, tc.elapsed)
		if got < tc.want-0.01 || got > tc.want+0.01 {
			t.Errorf("elapsed=%v: got %.4f, want %.4f", tc.elapsed, got, tc.want)
		}
	}
}

func TestDecayStep(t *testing.T) {
	policy := DecayPolicy{Kind: "step", StepAt: 7 * 24 * time.Hour, StepTo: 0.3}
	if got := policy.ApplyDecay(0.9, 6*24*time.Hour); got != 0.9 {
		t.Errorf("before step: got %.2f want 0.9", got)
	}
	if got := policy.ApplyDecay(0.9, 8*24*time.Hour); got != 0.3 {
		t.Errorf("after step: got %.2f want 0.3", got)
	}
}

func TestStoreBasic(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(medicalFrame)

	now := time.Now()

	// Add a record
	err := s.Assert(&Record{
		ID:        "bp-reading-001",
		Content:   "Blood pressure 140/90 at 2pm",
		Timestamp: now,
		Frame:     "medical_diagnosis",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add a belief derived from the record
	err = s.Believe(&Belief{
		ID:         "hypertension-belief-001",
		Content:    "Patient likely has hypertension",
		Confidence: 0.82,
		AssertedAt: now,
		Frame:      "medical_diagnosis",
		Derivation: []string{"bp-reading-001"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Query immediately: should be ~0.82
	result, err := s.Query("hypertension-belief-001", now)
	if err != nil {
		t.Fatal(err)
	}
	if result.CurrentConfidence < 0.81 || result.CurrentConfidence > 0.83 {
		t.Errorf("immediate query: got %.4f want ~0.82", result.CurrentConfidence)
	}

	// Query after 30 days: should be ~0.41
	future := now.Add(30 * 24 * time.Hour)
	result, err = s.Query("hypertension-belief-001", future)
	if err != nil {
		t.Fatal(err)
	}
	if result.CurrentConfidence < 0.40 || result.CurrentConfidence > 0.42 {
		t.Errorf("30-day query: got %.4f want ~0.41", result.CurrentConfidence)
	}
	t.Logf("30-day confidence: %.4f", result.CurrentConfidence)
}

func TestRetractPoisonsBeliefs(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(medicalFrame)

	now := time.Now()

	s.Assert(&Record{ID: "r1", Content: "Observation 1", Timestamp: now, Frame: "medical_diagnosis"})
	s.Assert(&Record{ID: "r2", Content: "Observation 2", Timestamp: now, Frame: "medical_diagnosis"})

	s.Believe(&Belief{
		ID: "b1", Content: "Direct belief", Confidence: 0.8,
		AssertedAt: now, Frame: "medical_diagnosis", Derivation: []string{"r1"},
	})
	s.Believe(&Belief{
		ID: "b2", Content: "Downstream belief", Confidence: 0.7,
		AssertedAt: now, Frame: "medical_diagnosis", Derivation: []string{"b1", "r2"},
	})

	// Retract r1
	s.Retract("r1", "data entry error", now)

	// b1 should be suspect
	result, _ := s.Query("b1", now)
	if result.State != BeliefSuspect {
		t.Errorf("b1 should be suspect after r1 retracted, got state=%d", result.State)
	}
	// Suspect beliefs now return their decayed confidence, not 0.
	// Confidence=0 was conflating "untrustworthy" with "false". Check state instead.
	if result.CurrentConfidence <= 0 {
		t.Errorf("suspect belief should have positive decayed confidence, got %.4f", result.CurrentConfidence)
	}

	// b2 depends on b1, should also be suspect
	result, _ = s.Query("b2", now)
	if result.State != BeliefSuspect {
		t.Errorf("b2 should be suspect (downstream of b1), got state=%d", result.State)
	}
}

func TestCrossFrameImportedDecay(t *testing.T) {
	// The most_conservative policy: a belief derived from sensor data (fast decay)
	// and diagnostic reasoning (slow decay) should decay at the sensor rate.
	s := NewStore()
	s.RegisterFrame(medicalFrame)
	s.RegisterFrame(sensorFrame)

	now := time.Now()

	// Sensor reading
	s.Assert(&Record{ID: "temp-sensor-001", Content: "Body temp 38.5C", Timestamp: now, Frame: "sensor_fusion"})

	// Sensor-frame belief
	s.Believe(&Belief{
		ID:         "fever-sensor-belief",
		Content:    "Sensor indicates fever",
		Confidence: 0.95,
		AssertedAt: now,
		Frame:      "sensor_fusion",
		Derivation: []string{"temp-sensor-001"},
	})

	// Medical-frame belief derived from both: carries sensor frame's fast decay
	s.Assert(&Record{ID: "history-001", Content: "No fever history in last 6 months", Timestamp: now, Frame: "medical_diagnosis"})
	s.Believe(&Belief{
		ID:         "fever-diagnosis-belief",
		Content:    "Patient has fever, likely viral",
		Confidence: 0.88,
		AssertedAt: now,
		Frame:      "medical_diagnosis",
		Derivation: []string{"fever-sensor-belief", "history-001"},
	})

	// Snapshot semantics: the sensor belief's confidence (0.95) was snapshotted at
	// assertion time. Only the medical frame's 30d halflife applies going forward.
	// After 1 hour: medical decay ≈ 1.0 (30d halflife >> 1h elapsed).
	// The snapshot bound is 0.95; own confidence 0.88 is the binding limit.
	// Expected: ~0.88 (medical decay barely moves over 1 hour)
	oneHour := now.Add(1 * time.Hour)
	result, err := s.Query("fever-diagnosis-belief", oneHour)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cross-frame belief after 1 hour: %.4f (snapshot semantics: only medical decay)", result.CurrentConfidence)

	if result.CurrentConfidence < 0.87 {
		t.Errorf("cross-frame 1h: got %.4f, expected ~0.88 (snapshot: only 30d medical decay applied)", result.CurrentConfidence)
	}
}

func TestBeliefChainDecay(t *testing.T) {
	// Three-step derivation chain: does decay compound correctly?
	s := NewStore()
	s.RegisterFrame(medicalFrame)
	now := time.Now()

	s.Assert(&Record{ID: "raw-001", Content: "Raw observation", Timestamp: now, Frame: "medical_diagnosis"})
	s.Believe(&Belief{
		ID: "step1", Content: "First inference", Confidence: 0.9,
		AssertedAt: now, Frame: "medical_diagnosis", Derivation: []string{"raw-001"},
	})
	s.Believe(&Belief{
		ID: "step2", Content: "Second inference", Confidence: 0.85,
		AssertedAt: now, Frame: "medical_diagnosis", Derivation: []string{"step1"},
	})
	s.Believe(&Belief{
		ID: "step3", Content: "Final conclusion", Confidence: 0.8,
		AssertedAt: now, Frame: "medical_diagnosis", Derivation: []string{"step2"},
	})

	// Same-frame chain: imported decay doesn't apply (same frame).
	// Each belief decays by its own frame policy.
	future := now.Add(30 * 24 * time.Hour)
	for _, id := range []string{"step1", "step2", "step3"} {
		r, _ := s.Query(id, future)
		t.Logf("%s after 30d: %.4f", id, r.CurrentConfidence)
	}

	// All same frame, all assertedAt same time — they should all decay identically
	// relative to their original confidence. step3 should be ~0.4 (0.8 * 0.5)
	r, _ := s.Query("step3", future)
	if r.CurrentConfidence < 0.39 || r.CurrentConfidence > 0.41 {
		t.Errorf("step3 after 30d: got %.4f want ~0.40", r.CurrentConfidence)
	}
}

func ExampleStore() {
	s := NewStore()
	s.RegisterFrame(Frame{
		Name:        "example",
		Composition: "bayesian",
		Decay: DecayPolicy{
			Kind:     "exponential",
			Halflife: 7 * 24 * time.Hour,
		},
		ImportedDecayPolicy: "most_conservative",
	})

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Assert(&Record{ID: "r1", Content: "Evidence", Timestamp: t0, Frame: "example"})
	s.Believe(&Belief{
		ID: "b1", Content: "Conclusion", Confidence: 0.9,
		AssertedAt: t0, Frame: "example", Derivation: []string{"r1"},
	})

	// Query after 7 days (one halflife)
	r, _ := s.Query("b1", t0.Add(7*24*time.Hour))
	fmt.Printf("Confidence after 7 days: %.2f\n", r.CurrentConfidence)
	// Output: Confidence after 7 days: 0.45
}


