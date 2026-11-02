package observability

import (
	"math"
	"sort"
)

// Summary describes a sample of measurements. P95 and P99 are nil when the
// sample is too small for the percentile to be meaningful.
type Summary struct {
	N        int      `json:"n"`
	Mean     float64  `json:"mean"`
	StdDev   float64  `json:"std_dev"`
	Median   float64  `json:"median"`
	Min      float64  `json:"min"`
	Max      float64  `json:"max"`
	P95      *float64 `json:"p95"`
	P99      *float64 `json:"p99"`
	CI95Half *float64 `json:"ci95_half"`
}

// Minimum sample sizes before tail percentiles are reported.
const (
	minSamplesP95 = 20
	minSamplesP99 = 100
)

// Percentile returns the p-th percentile (0..100) of an ascending-sorted
// slice using linear interpolation. It returns NaN for an empty slice.
func Percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := p / 100 * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// Summarize computes summary statistics. It returns false for an empty sample.
func Summarize(values []float64) (Summary, bool) {
	n := len(values)
	if n == 0 {
		return Summary{}, false
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	mean := sum / float64(n)
	s := Summary{
		N:      n,
		Mean:   mean,
		Median: Percentile(sorted, 50),
		Min:    sorted[0],
		Max:    sorted[n-1],
	}
	if n >= 2 {
		ss := 0.0
		for _, v := range sorted {
			d := v - mean
			ss += d * d
		}
		s.StdDev = math.Sqrt(ss / float64(n-1))
		half := tCritical95(n-1) * s.StdDev / math.Sqrt(float64(n))
		s.CI95Half = &half
	}
	if n >= minSamplesP95 {
		v := Percentile(sorted, 95)
		s.P95 = &v
	}
	if n >= minSamplesP99 {
		v := Percentile(sorted, 99)
		s.P99 = &v
	}
	return s, true
}

// Two-sided 95% Student-t critical values for 1..30 degrees of freedom.
var tTable = [...]float64{
	12.706, 4.303, 3.182, 2.776, 2.571, 2.447, 2.365, 2.306, 2.262, 2.228,
	2.201, 2.179, 2.160, 2.145, 2.131, 2.120, 2.110, 2.101, 2.093, 2.086,
	2.080, 2.074, 2.069, 2.064, 2.060, 2.056, 2.052, 2.048, 2.045, 2.042,
}

func tCritical95(df int) float64 {
	if df < 1 {
		return math.NaN()
	}
	if df <= len(tTable) {
		return tTable[df-1]
	}
	return 1.96
}
