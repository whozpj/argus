package drift

import "math"

// mannWhitneyU computes the U statistic for sample x against sample y.
// Counts how often an x value exceeds a y value (ties count as 0.5).
// O(m*n) — fine for the sample sizes we use (200 baseline, 50 recent).
func mannWhitneyU(x, y []float64) float64 {
	var u float64
	for _, xi := range x {
		for _, yj := range y {
			if xi > yj {
				u += 1.0
			} else if xi == yj {
				u += 0.5
			}
		}
	}
	return u
}

// mannWhitneyPValue returns the two-tailed p-value for the Mann-Whitney U test
// using the normal approximation. Returns 1.0 if either sample is empty.
func mannWhitneyPValue(x, y []float64) float64 {
	m := float64(len(x))
	n := float64(len(y))
	if m == 0 || n == 0 {
		return 1.0
	}

	u := mannWhitneyU(x, y)
	mu := m * n / 2.0
	sigma := math.Sqrt(m * n * (m + n + 1) / 12.0)
	if sigma == 0 {
		return 1.0
	}

	z := math.Abs(u-mu) / sigma
	// Two-tailed: P(|Z| > z) = erfc(z / √2)
	return math.Erfc(z / math.Sqrt2)
}

// alpha is the family-wise error rate target for Bonferroni correction.
// Score is calibrated so that score > 0 means Bonferroni-significant at α=0.05,
// and score = 1 means p ≈ 0.
const alpha = 0.05

// driftScore converts per-signal p-values into a single [0, 1] score using
// Bonferroni correction, calibrated to α = 0.05.
//
// Method:
//   corrected_p = min(p_i) * k          (Bonferroni)
//   score       = 1 - corrected_p / α   clamped to [0, 1]
//
// score = 0 means no evidence of drift (corrected_p ≥ α).
// score = 1 means p ≈ 0 (extreme drift).
// The alert threshold of 0.7 corresponds to corrected_p < 0.015 (< α×0.3),
// giving a per-window false positive rate of ~1.5% for two independent signals.
func driftScore(pValues []float64) float64 {
	if len(pValues) == 0 {
		return 0
	}
	k := float64(len(pValues))
	minP := 1.0
	for _, p := range pValues {
		if p < minP {
			minP = p
		}
	}
	corrected := minP * k
	score := 1.0 - corrected/alpha
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}
