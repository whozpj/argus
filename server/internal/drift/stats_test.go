package drift

import (
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// mannWhitneyU
// ---------------------------------------------------------------------------

func TestMannWhitneyU_AllXGreater(t *testing.T) {
	// Every x beats every y → U = m*n
	x := []float64{10, 20, 30}
	y := []float64{1, 2, 3}
	want := float64(len(x) * len(y)) // 9
	if got := mannWhitneyU(x, y); got != want {
		t.Errorf("mannWhitneyU = %v, want %v", got, want)
	}
}

func TestMannWhitneyU_AllYGreater(t *testing.T) {
	// Every y beats every x → U = 0
	x := []float64{1, 2, 3}
	y := []float64{10, 20, 30}
	if got := mannWhitneyU(x, y); got != 0 {
		t.Errorf("mannWhitneyU = %v, want 0", got)
	}
}

func TestMannWhitneyU_AllTied(t *testing.T) {
	// All values equal → each pair contributes 0.5 → U = m*n*0.5
	x := []float64{5, 5, 5}
	y := []float64{5, 5, 5}
	want := float64(len(x)*len(y)) * 0.5 // 4.5
	if got := mannWhitneyU(x, y); got != want {
		t.Errorf("mannWhitneyU = %v, want %v", got, want)
	}
}

func TestMannWhitneyU_Symmetry(t *testing.T) {
	// Ux + Uy = m*n
	x := []float64{3, 1, 4, 1, 5}
	y := []float64{9, 2, 6}
	ux := mannWhitneyU(x, y)
	uy := mannWhitneyU(y, x)
	mn := float64(len(x) * len(y))
	if math.Abs(ux+uy-mn) > 1e-9 {
		t.Errorf("Ux(%v) + Uy(%v) = %v, want %v", ux, uy, ux+uy, mn)
	}
}

// ---------------------------------------------------------------------------
// mannWhitneyPValue
// ---------------------------------------------------------------------------

func TestMannWhitneyPValue_IdenticalDistributions(t *testing.T) {
	// Same data → U ≈ m*n/2 → z ≈ 0 → p ≈ 1
	x := make([]float64, 50)
	y := make([]float64, 50)
	for i := range x {
		x[i] = float64(i)
		y[i] = float64(i)
	}
	p := mannWhitneyPValue(x, y)
	if p < 0.9 {
		t.Errorf("p = %v for identical distributions, expected > 0.9", p)
	}
}

func TestMannWhitneyPValue_ClearlyDifferent(t *testing.T) {
	// Non-overlapping distributions → should give very small p-value
	x := make([]float64, 50) // baseline: 0..49
	y := make([]float64, 50) // recent: 200..249
	for i := range x {
		x[i] = float64(i)
		y[i] = float64(i + 200)
	}
	p := mannWhitneyPValue(x, y)
	if p > 0.001 {
		t.Errorf("p = %v for clearly different distributions, expected < 0.001", p)
	}
}

func TestMannWhitneyPValue_EmptySample(t *testing.T) {
	p := mannWhitneyPValue([]float64{1, 2, 3}, []float64{})
	if p != 1.0 {
		t.Errorf("p = %v for empty sample, want 1.0", p)
	}
}

func TestMannWhitneyPValue_InRange(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	y := []float64{3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	p := mannWhitneyPValue(x, y)
	if p < 0 || p > 1 {
		t.Errorf("p = %v is outside [0, 1]", p)
	}
}

// ---------------------------------------------------------------------------
// driftScore
// ---------------------------------------------------------------------------

func TestDriftScore_NoPValues(t *testing.T) {
	if got := driftScore(nil); got != 0 {
		t.Errorf("driftScore(nil) = %v, want 0", got)
	}
}

func TestDriftScore_HighPValues(t *testing.T) {
	// p close to 1 → score close to 0 (no drift evidence)
	score := driftScore([]float64{0.9, 0.85})
	if score > 0.3 {
		t.Errorf("score = %v for high p-values, expected < 0.3", score)
	}
}

func TestDriftScore_LowPValues(t *testing.T) {
	// p close to 0 → score close to 1 (strong drift evidence)
	score := driftScore([]float64{0.001, 0.5})
	if score < 0.7 {
		t.Errorf("score = %v for low p-value, expected > 0.7", score)
	}
}

func TestDriftScore_BonferroniCapsAt1(t *testing.T) {
	// Very small p × k could exceed 1 before capping
	score := driftScore([]float64{0.0001, 0.0001})
	if score < 0 || score > 1 {
		t.Errorf("score = %v is outside [0, 1]", score)
	}
}

func TestDriftScore_UsesMinPValue(t *testing.T) {
	// Score should be driven by the most significant (smallest) p-value
	score1 := driftScore([]float64{0.01, 0.9})  // min=0.01
	score2 := driftScore([]float64{0.01, 0.01}) // min=0.01 — same min, same score
	if math.Abs(score1-score2) > 1e-9 {
		t.Errorf("scores differ (%v vs %v) but min p-value is the same", score1, score2)
	}
}

func TestDriftScore_AboveThresholdForClearDrift(t *testing.T) {
	// A p-value of 0.001 with k=2: corrected = 0.002 → score = 0.998
	score := driftScore([]float64{0.001, 0.8})
	if score < 0.7 {
		t.Errorf("score = %v, expected > 0.7 for p=0.001 with Bonferroni k=2", score)
	}
}
