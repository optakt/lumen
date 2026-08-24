package lumen

import (
	"strings"
	"testing"
	"time"
)

func TestReflectCalibration(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(medicalFrame)
	s.RegisterFrame(sensorFrame)

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	s.Assert(&Record{ID: "r1", Content: "Lab result", Timestamp: t0, Frame: "medical_diagnosis"})
	s.Assert(&Record{ID: "r2", Content: "Patient history", Timestamp: t0, Frame: "medical_diagnosis"})
	s.Assert(&Record{ID: "temp", Content: "Sensor temp", Timestamp: t0, Frame: "sensor_fusion"})
	s.Believe(&Belief{
		ID: "fever", Content: "Fever detected", Confidence: 0.94,
		AssertedAt: t0, Frame: "sensor_fusion", Derivation: []string{"temp"},
	})
	// Well-grounded belief: two records + cross-frame
	s.Believe(&Belief{
		ID: "diag", Content: "Complex diagnosis", Confidence: 0.85,
		AssertedAt: t0, Frame: "medical_diagnosis", Derivation: []string{"r1", "r2", "fever"},
	})
	// Poorly grounded: single source, overconfident
	s.Believe(&Belief{
		ID: "guess", Content: "Maybe autoimmune", Confidence: 0.98,
		AssertedAt: t0, Frame: "medical_diagnosis", Derivation: []string{"r1"},
	})

	// Calibration check on well-grounded belief immediately
	ans, err := s.Reflect(ReflectiveQuery{"diag", "is_well_calibrated"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + FormatAnswer(ans))

	// Calibration check on overconfident belief
	ans, err = s.Reflect(ReflectiveQuery{"guess", "is_well_calibrated"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + FormatAnswer(ans))
	if ans.MetaConfidence > 0.65 {
		t.Errorf("overconfident single-source belief should have lower meta-confidence, got %.2f", ans.MetaConfidence)
	}

	// After 2 hours: sensor decay should trigger warning
	ans, err = s.Reflect(ReflectiveQuery{"diag", "is_well_calibrated"}, t0.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + FormatAnswer(ans))
	if !strings.Contains(strings.Join(ans.Observations, " "), "imported decay") {
		t.Error("should warn about imported decay dominating")
	}
}

func TestReflectShouldUpdate(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(sensorFrame)

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Assert(&Record{ID: "r1", Content: "Sensor", Timestamp: t0, Frame: "sensor_fusion"})
	s.Believe(&Belief{
		ID: "b1", Content: "Current state", Confidence: 0.9,
		AssertedAt: t0, Frame: "sensor_fusion", Derivation: []string{"r1"},
	})

	// Immediately: no update needed
	ans, _ := s.Reflect(ReflectiveQuery{"b1", "should_update"}, t0)
	t.Log("\n" + FormatAnswer(ans))
	if ans.Answer != "no update needed" {
		t.Errorf("expected no update needed immediately, got: %s", ans.Answer)
	}

	// After 5 hours (5 halflives of 1h sensor): update recommended
	ans, _ = s.Reflect(ReflectiveQuery{"b1", "should_update"}, t0.Add(5*time.Hour))
	t.Log("\n" + FormatAnswer(ans))
	if ans.Answer != "update recommended" {
		t.Errorf("expected update recommended after 5h, got: %s", ans.Answer)
	}
}

func TestReflectWhatWouldChange(t *testing.T) {
	s := NewStore()
	s.RegisterFrame(medicalFrame)
	s.RegisterFrame(sensorFrame)

	t0 := time.Now()
	s.Assert(&Record{ID: "r1", Content: "Lab test", Timestamp: t0, Frame: "medical_diagnosis"})
	s.Assert(&Record{ID: "temp1", Content: "Temp sensor", Timestamp: t0, Frame: "sensor_fusion"})
	s.Believe(&Belief{
		ID: "sensor-belief", Content: "Fever", Confidence: 0.9,
		AssertedAt: t0, Frame: "sensor_fusion", Derivation: []string{"temp1"},
	})
	s.Believe(&Belief{
		ID: "diag", Content: "Febrile condition", Confidence: 0.8,
		AssertedAt: t0, Frame: "medical_diagnosis", Derivation: []string{"r1", "sensor-belief"},
	})

	ans, err := s.Reflect(ReflectiveQuery{"diag", "what_would_change_this"}, t0.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + FormatAnswer(ans))

	// Should mention the cross-frame imported decay issue
	obsText := strings.Join(ans.Observations, " ")
	if !strings.Contains(obsText, "imported decay") {
		t.Error("should mention imported decay as a change condition")
	}
}
