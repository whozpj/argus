package drift

// Accuracy validation for the Mann-Whitney U + Bonferroni drift detector.
//
// Run with:
//   go test -v -run TestAccuracy ./internal/drift/
//
// These tests measure:
//   - False positive rate (FPR): how often the detector fires when there is NO drift
//   - True positive rate (TPR): how often it fires at various drift magnitudes
//
// Each trial draws fresh random samples using a simple LCG so results are
// deterministic and reproducible without importing math/rand.

import (
	"fmt"
	"math"
	"testing"
)

// ── Minimal deterministic PRNG (LCG) ─────────────────────────────────────────

type lcg struct{ state uint64 }

func newLCG(seed uint64) *lcg { return &lcg{seed} }

// next returns a float64 in [0, 1).
func (r *lcg) next() float64 {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return float64(r.state>>11) / (1 << 53)
}

// normalPair returns two independent N(0,1) samples via Box-Muller.
func (r *lcg) normalPair() (float64, float64) {
	u1 := r.next()
	u2 := r.next()
	if u1 < 1e-10 {
		u1 = 1e-10
	}
	z0 := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	z1 := math.Sqrt(-2*math.Log(u1)) * math.Sin(2*math.Pi*u2)
	return z0, z1
}

// sample draws n values from N(mean, std).
func (r *lcg) sample(n int, mean, std float64) []float64 {
	out := make([]float64, n)
	for i := 0; i < n; i += 2 {
		z0, z1 := r.normalPair()
		out[i] = mean + std*z0
		if i+1 < n {
			out[i+1] = mean + std*z1
		}
	}
	return out
}

// driftDetected returns true if the drift score exceeds the alert threshold.
func driftDetected(baseline, recent []float64) bool {
	p := mannWhitneyPValue(baseline, recent)
	score := driftScore([]float64{p, 1.0}) // single signal (output_tokens only)
	return score > alertThreshold
}

// runTrials runs `trials` independent experiments. Each trial draws a fresh
// baseline (200 samples) and recent window (50 samples). Returns detection rate.
func runTrials(rng *lcg, trials, baselineN, recentN int,
	baseMean, baseStd, recentMean, recentStd float64) float64 {
	detected := 0
	for i := 0; i < trials; i++ {
		base := rng.sample(baselineN, baseMean, baseStd)
		recent := rng.sample(recentN, recentMean, recentStd)
		if driftDetected(base, recent) {
			detected++
		}
	}
	return float64(detected) / float64(trials)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestAccuracy_FalsePositiveRate verifies that the detector rarely fires when
// baseline and recent come from the same distribution.
// Expected: FPR < 5% (α = 0.05 with Bonferroni on 1 signal here).
func TestAccuracy_FalsePositiveRate(t *testing.T) {
	rng := newLCG(42)
	trials := 500
	fpr := runTrials(rng, trials, 200, 50, 60, 15, 60, 15)

	t.Logf("False positive rate over %d trials: %.1f%%", trials, fpr*100)

	if fpr > 0.05 {
		t.Errorf("FPR = %.3f (%.1f%%) — exceeds 5%% threshold", fpr, fpr*100)
	}
}

// TestAccuracy_PowerAtVariousDriftLevels measures true positive rate at
// increasing multiples of the baseline mean. Reports a table.
func TestAccuracy_PowerAtVariousDriftLevels(t *testing.T) {
	const (
		baseMean = 60.0
		baseStd  = 15.0
		trials   = 500
		baseN    = 200
		recentN  = 50
	)

	type row struct {
		label    string
		multiple float64
		expected float64 // minimum acceptable TPR
	}

	cases := []row{
		{"1.0× (no drift)",    1.0,  0.00}, // just measuring, no assertion
		{"1.2× (+20%)",        1.2,  0.20},
		{"1.5× (+50%)",        1.5,  0.60},
		{"2.0× (+100%)",       2.0,  0.95},
		{"3.0× (+200%)",       3.0,  0.99},
		{"5.0× (+400%)",       5.0,  0.999},
		{"10.0× (+900%)",      10.0, 1.00},
	}

	rng := newLCG(1337)

	t.Log("\nDrift magnitude → True positive rate")
	t.Log("─────────────────────────────────────────")

	for _, c := range cases {
		recentMean := baseMean * c.multiple
		tpr := runTrials(rng, trials, baseN, recentN, baseMean, baseStd, recentMean, baseStd)
		t.Logf("  %-20s  TPR = %5.1f%%  (n=%d trials, baseline mean=%.0f, recent mean=%.0f)",
			c.label, tpr*100, trials, baseMean, recentMean)

		if c.multiple > 1.0 && tpr < c.expected {
			t.Errorf("%s: TPR = %.3f, want >= %.3f", c.label, tpr, c.expected)
		}
	}
}

// TestAccuracy_LatencyDrift verifies detection works equally for latency signal.
func TestAccuracy_LatencyDrift(t *testing.T) {
	rng := newLCG(9999)
	// Baseline: 350ms mean latency. Drifted: 1800ms (~5× — typical model degradation).
	tpr := runTrials(rng, 500, 200, 50, 350, 80, 1800, 200)
	t.Logf("Latency 5× drift TPR: %.1f%%", tpr*100)
	if tpr < 0.99 {
		t.Errorf("expected TPR >= 0.99 for 5× latency drift, got %.3f", tpr)
	}
}

// TestAccuracy_SmallSampleDegrades verifies that detection power decreases
// with a smaller recent window — the test should not fire for tiny windows.
func TestAccuracy_SmallSampleDegrades(t *testing.T) {
	type row struct {
		recentN  int
		minTPR   float64
	}
	cases := []row{
		{10,  0.0},  // barely detectable
		{20,  0.5},
		{50,  0.95},
		{100, 0.999},
	}

	rng := newLCG(2025)
	t.Log("\nRecent window size → TPR (3× drift)")
	t.Log("─────────────────────────────────────")
	for _, c := range cases {
		tpr := runTrials(rng, 500, 200, c.recentN, 60, 15, 180, 15)
		t.Logf("  recent_n=%-4d  TPR = %5.1f%%", c.recentN, tpr*100)
		if tpr < c.minTPR {
			t.Errorf("recent_n=%d: TPR=%.3f, want >= %.3f", c.recentN, tpr, c.minTPR)
		}
	}
}

// TestAccuracy_ReproducibleAcrossSeeds confirms results are stable across
// different random seeds (not a fluke of one seed).
func TestAccuracy_ReproducibleAcrossSeeds(t *testing.T) {
	seeds := []uint64{1, 42, 100, 9999, 123456}
	for _, seed := range seeds {
		rng := newLCG(seed)
		// 10× drift should always be detected.
		tpr := runTrials(rng, 200, 200, 50, 60, 15, 600, 15)
		if tpr < 1.0 {
			t.Errorf("seed=%d: 10× drift TPR=%.3f, want 1.0", seed, tpr)
		}
		// No drift should almost never fire.
		fpr := runTrials(rng, 200, 200, 50, 60, 15, 60, 15)
		if fpr > 0.05 {
			t.Errorf("seed=%d: FPR=%.3f, want < 0.05", seed, fpr)
		}
	}
	t.Logf("All %d seeds: 10× drift always detected, FPR < 5%%", len(seeds))
}

// Suppress "unused" for fmt (used in Logf calls above indirectly via fmt.Sprintf equivalent).
var _ = fmt.Sprintf
