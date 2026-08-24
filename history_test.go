package lumen

import (
	"strings"
	"testing"
	"time"
)

func TestEpistemicTrace(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(medicalFrame)
	s.RegisterFrame(sensorFrame)

	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	s.Assert(&Record{ID: "bp001", Content: "BP 140/90", Timestamp: t0, Frame: "medical_diagnosis"})
	s.Assert(&Record{ID: "temp001", Content: "Temp 38.5C", Timestamp: t0, Frame: "sensor_fusion"})
	s.Believe(&Belief{
		ID: "fever001", Content: "Sensor indicates fever",
		Confidence: 0.93, AssertedAt: t0, Frame: "sensor_fusion",
		Derivation: []string{"temp001"},
	})
	s.Believe(&Belief{
		ID: "diagnosis001", Content: "Hypertensive fever, likely viral",
		Confidence: 0.85, AssertedAt: t0, Frame: "medical_diagnosis",
		Derivation: []string{"bp001", "fever001"},
	})

	// Trace immediately
	trace, err := s.EpistemicTrace("diagnosis001", t0)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + trace)

	if !strings.Contains(trace, "cross-frame from sensor_fusion") {
		t.Error("trace should mention cross-frame source")
	}

	// Trace after 2 hours (sensor decay significant)
	trace, err = s.EpistemicTrace("diagnosis001", t0.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + trace)

	// After 30 days
	trace, err = s.EpistemicTrace("diagnosis001", t0.Add(30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + trace)
}

func TestEpistemicTraceWithRetraction(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(medicalFrame)

	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(3 * 24 * time.Hour) // 3 days later

	s.Assert(&Record{ID: "r1", Content: "Lab test result", Timestamp: t0, Frame: "medical_diagnosis"})
	s.Believe(&Belief{
		ID: "b1", Content: "Elevated markers", Confidence: 0.9,
		AssertedAt: t0, Frame: "medical_diagnosis", Derivation: []string{"r1"},
	})

	s.Retract("r1", "sample contamination", t1)

	trace, err := s.EpistemicTrace("b1", t1.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + trace)

	if !strings.Contains(trace, "RETRACTED") {
		t.Error("trace should show retraction")
	}
	if !strings.Contains(trace, "SUSPECT") {
		t.Error("trace should show belief is suspect")
	}
}

func TestWhatChangedMyMind(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(sensorFrame) // fast decay, 1h halflife

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Assert(&Record{ID: "r1", Content: "Sensor reading", Timestamp: t0, Frame: "sensor_fusion"})
	s.Believe(&Belief{
		ID: "b1", Content: "Sensor belief", Confidence: 0.9,
		AssertedAt: t0, Frame: "sensor_fusion", Derivation: []string{"r1"},
	})

	// Look for drops of > 0.05 over a 6-hour window, sampled every 30 min
	changes, err := s.WhatChangedMyMind("b1", t0, t0.Add(6*time.Hour), 0.05, 12)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range changes {
		t.Logf("%.2f → %.2f at %s (%s)", ch.OldConf, ch.NewConf,
			ch.At.Format("15:04"), ch.Reason)
	}
	if len(changes) == 0 {
		t.Error("expected some confidence changes over 6 hours with 1h halflife")
	}
}
