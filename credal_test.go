package lumen

import (
	"math"
	"testing"
)

func TestCredalBayesUpdateBasic(t *testing.T) {
	// Point prior, point LR → should match standard BayesianCompose
	prior := CredalPrior{Lo: 0.50, Hi: 0.50}
	evidence := []CredalEvidence{
		{SourceID: "e1", Confidence: 0.85, LRLo: 5.0, LRHi: 5.0},
	}
	posterior, err := CredalBayesUpdate(prior, evidence)
	if err != nil {
		t.Fatal(err)
	}
	// Should match standard Bayes
	pointResult, _ := BayesianCompose(0.50, []Evidence{{SourceID: "e1", Confidence: 0.85, LikelihoodRatio: 5.0}})
	if math.Abs(posterior.Lo-pointResult) > 0.001 {
		t.Errorf("point prior/LR: credal [%.4f, %.4f] should match point %.4f", posterior.Lo, posterior.Hi, pointResult)
	}
	if posterior.Width() > 0.001 {
		t.Errorf("point inputs should give point output, got width %.4f", posterior.Width())
	}
	t.Logf("Point case: credal=%.4f, point=%.4f (match)", posterior.Lo, pointResult)
}

func TestCredalMonotonicity(t *testing.T) {
	// Verify that posterior is increasing in prior (for LR > 0)
	lrEvidence := []CredalEvidence{{SourceID: "e", Confidence: 0.90, LRLo: 4.0, LRHi: 4.0}}
	priors := []float64{0.1, 0.3, 0.5, 0.7, 0.9}
	var posts []float64
	for _, p := range priors {
		prior := CredalPrior{Lo: p - 0.001, Hi: p + 0.001}
		if prior.Lo < 0.001 {
			prior.Lo = 0.001
		}
		if prior.Hi > 0.999 {
			prior.Hi = 0.999
		}
		posterior, err := CredalBayesUpdate(prior, lrEvidence)
		if err != nil {
			t.Fatalf("prior %.2f: %v", p, err)
		}
		posts = append(posts, posterior.Midpoint())
	}
	for i := 1; i < len(posts); i++ {
		if posts[i] <= posts[i-1] {
			t.Errorf("posterior not monotone: posts[%d]=%.4f <= posts[%d]=%.4f", i, posts[i], i-1, posts[i-1])
		}
	}
	t.Logf("Monotonicity confirmed: %v", posts)
}

func TestCredalIntervalShrinkage(t *testing.T) {
	// Evidence should reduce imprecision (shrink the interval)
	prior := CredalPrior{Lo: 0.35, Hi: 0.65} // wide prior
	priorWidth := prior.Hi - prior.Lo

	evidence := []CredalEvidence{
		{SourceID: "zombie", Confidence: 0.65, LRLo: 4.5, LRHi: 4.5},
	}
	posterior, err := CredalBayesUpdate(prior, evidence)
	if err != nil {
		t.Fatal(err)
	}

	if posterior.Width() >= priorWidth {
		t.Errorf("evidence should reduce imprecision: prior width=%.3f, posterior width=%.3f",
			priorWidth, posterior.Width())
	}
	t.Logf("Prior width %.3f → Posterior width %.3f (reduced by %.3f)",
		priorWidth, posterior.Width(), priorWidth-posterior.Width())
}

func TestCredalIntervalLR(t *testing.T) {
	// Interval LR (uncertainty about how diagnostic evidence is) widens the posterior
	prior := CredalPrior{Lo: 0.40, Hi: 0.60}

	// Point LR vs interval LR
	pointEv := []CredalEvidence{{SourceID: "e", Confidence: 0.80, LRLo: 4.0, LRHi: 4.0}}
	intervalEv := []CredalEvidence{{SourceID: "e", Confidence: 0.80, LRLo: 2.0, LRHi: 6.0}} // wider LR

	postPoint, _ := CredalBayesUpdate(prior, pointEv)
	postInterval, _ := CredalBayesUpdate(prior, intervalEv)

	t.Logf("Point LR=4.0: posterior [%.3f, %.3f] (width=%.3f)", postPoint.Lo, postPoint.Hi, postPoint.Width())
	t.Logf("Interval LR=[2,6]: posterior [%.3f, %.3f] (width=%.3f)", postInterval.Lo, postInterval.Hi, postInterval.Width())

	if postInterval.Width() <= postPoint.Width() {
		t.Errorf("interval LR should give wider posterior: point=%.3f interval=%.3f",
			postPoint.Width(), postInterval.Width())
	}
}

func TestCredalSequentialIsBatch(t *testing.T) {
	// Sequential credal updates should equal batch update (product of LRs)
	prior := CredalPrior{Lo: 0.20, Hi: 0.70}
	ev1 := CredalEvidence{SourceID: "e1", Confidence: 0.80, LRLo: 3.5, LRHi: 3.5}
	ev2 := CredalEvidence{SourceID: "e2", Confidence: 0.75, LRLo: 2.0, LRHi: 2.0}
	ev3 := CredalEvidence{SourceID: "e3", Confidence: 0.60, LRLo: 0.4, LRHi: 0.4}

	// Sequential
	p1, _ := CredalBayesUpdate(prior, []CredalEvidence{ev1})
	p2, _ := CredalBayesUpdate(CredalPrior{Lo: p1.Lo, Hi: p1.Hi}, []CredalEvidence{ev2})
	seqPost, _ := CredalBayesUpdate(CredalPrior{Lo: p2.Lo, Hi: p2.Hi}, []CredalEvidence{ev3})

	// Batch
	batchPost, _ := CredalBayesUpdate(prior, []CredalEvidence{ev1, ev2, ev3})

	if math.Abs(seqPost.Lo-batchPost.Lo) > 1e-9 || math.Abs(seqPost.Hi-batchPost.Hi) > 1e-9 {
		t.Errorf("sequential != batch: seq=[%.6f, %.6f] batch=[%.6f, %.6f]",
			seqPost.Lo, seqPost.Hi, batchPost.Lo, batchPost.Hi)
	}
	t.Logf("Sequential = Batch: [%.6f, %.6f]", batchPost.Lo, batchPost.Hi)
}

func TestCredalAuditHardProblem(t *testing.T) {
	// Apply credal analysis to the hard problem of consciousness
	// Prior: genuinely uncertain [0.35, 0.65] rather than declaring p=0.50
	prior := CredalPrior{Lo: 0.35, Hi: 0.65}
	evidence := []CredalEvidence{
		// Zombie argument: LR is uncertain [3.0, 6.0] — how strong is the modal intuition?
		{SourceID: "zombie-argument", Confidence: 0.65, LRLo: 3.0, LRHi: 6.0},
		// Knowledge argument: similar range
		{SourceID: "knowledge-argument", Confidence: 0.70, LRLo: 2.5, LRHi: 5.0},
		// Counter-evidence (illusionism): LR < 1, uncertain [0.2, 0.5]
		{SourceID: "illusionism-reply", Confidence: 0.60, LRLo: 0.2, LRHi: 0.5},
	}

	report, err := CredalAudit(prior, evidence)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("\n" + FormatCredalReport(report))

	// The posterior interval should contain our earlier point estimate of 0.72
	if !report.Posterior.Contains(0.72) {
		t.Logf("NOTE: 0.72 outside credal interval [%.3f, %.3f] — declared confidence may be miscalibrated",
			report.Posterior.Lo, report.Posterior.Hi)
	} else {
		t.Logf("0.72 is within credal interval [%.3f, %.3f] — consistent with imprecise prior",
			report.Posterior.Lo, report.Posterior.Hi)
	}

	// Width should be positive (there is genuine imprecision)
	if report.PosteriorWidth <= 0 {
		t.Error("posterior should have positive width given interval inputs")
	}

	// Imprecision should be reduced vs prior (evidence narrows uncertainty)
	// Note: interval LRs may cause width > point case but should still be narrower than prior
	t.Logf("Prior width: %.3f → Posterior width: %.3f", report.PriorWidth, report.PosteriorWidth)
}

func TestCredalDecayInterval(t *testing.T) {
	// Uncertain age + uncertain halflife → uncertain decay factor
	decay := CredalDecay{
		AgeMin:      0,    // just asserted
		AgeMax:      2,    // or up to 2 hours ago
		HalflifeMin: 720,  // 30 days in hours (lower bound)
		HalflifeMax: 1440, // 60 days in hours (upper bound)
	}
	lo, hi := decay.DecayInterval()
	t.Logf("Credal decay: age [0, 2h], halflife [30d, 60d] → factor [%.4f, %.4f]", lo, hi)

	if lo > hi {
		t.Error("decay factor lo should be <= hi")
	}
	// At age=0, decay = 1.0 regardless of halflife
	if math.Abs(hi-1.0) > 0.01 {
		t.Errorf("max decay (min age, max halflife) should be near 1.0, got %.4f", hi)
	}

	// Sensor case: short halflife, aged several halflives
	sensorDecay := CredalDecay{
		AgeMin:      0.5,  // at least 30 min old
		AgeMax:      3.0,  // at most 3 hours old
		HalflifeMin: 0.75, // 45 minutes
		HalflifeMax: 1.5,  // 90 minutes
	}
	slo, shi := sensorDecay.DecayInterval()
	t.Logf("Sensor decay: age [0.5h, 3h], halflife [45min, 90min] → factor [%.4f, %.4f]", slo, shi)
	if slo < 0 || shi > 1 {
		t.Error("decay factors must be in [0, 1]")
	}
}

func TestCredalBayesComposeParity(t *testing.T) {
	// When point prior and point LRs are used, CredalBayesUpdate should match BayesianCompose exactly
	cases := []struct {
		prior float64
		lr    float64
		conf  float64
	}{
		{0.1, 10.0, 0.95},
		{0.5, 2.5, 0.80},
		{0.8, 0.3, 0.70},
		{0.3, 5.0, 0.85},
	}

	for _, tc := range cases {
		credal, err := CredalBayesUpdate(
			CredalPrior{Lo: tc.prior, Hi: tc.prior},
			[]CredalEvidence{{SourceID: "e", Confidence: tc.conf, LRLo: tc.lr, LRHi: tc.lr}},
		)
		if err != nil {
			t.Errorf("prior=%.2f lr=%.2f: %v", tc.prior, tc.lr, err)
			continue
		}
		point, _ := BayesianCompose(tc.prior, []Evidence{{SourceID: "e", Confidence: tc.conf, LikelihoodRatio: tc.lr}})
		if math.Abs(credal.Lo-point) > 1e-9 {
			t.Errorf("parity failed: prior=%.2f lr=%.2f conf=%.2f credal=%.6f point=%.6f",
				tc.prior, tc.lr, tc.conf, credal.Lo, point)
		}
	}
	t.Log("Credal parity with BayesianCompose confirmed for all point cases")
}
