package runtime_test

import (
	"math"
	"runtime"
	"testing"
)

func TestScaleNanoseconds(t *testing.T) {
	for _, tc := range []struct {
		ns, multiplier, divisor, want int64
	}{
		{1, 64, 1953125, 0},
		{1953124, 64, 1953125, 63},
		{1000000000, 64, 1953125, 32768},
		{math.MaxInt64/64 + 1, 64, 1953125, 4722366482869},
		{math.MaxInt64, 64, 1953125, 302231454903657},
		{-math.MaxInt64, 64, 1953125, -302231454903657},
		{-1 << 63, 64, 1953125, -302231454903657},
		{math.MaxInt64, 39, 5000, 71942301887467251},
		{math.MaxInt64, 12, 625, 177088743107611695},
	} {
		if got := runtime.ScaleNanoseconds(tc.ns, tc.multiplier, tc.divisor); got != tc.want {
			t.Errorf("scale(%d, %d, %d) = %d, want %d", tc.ns, tc.multiplier, tc.divisor, got, tc.want)
		}
	}
}

func TestScaleNanosecondsFrequency(t *testing.T) {
	for _, tc := range []struct {
		ns        int64
		frequency uint64
		want      int64
	}{
		{0, 0, 0},
		{1_000_000_000, 3_000_000_000, 3_000_000_000},
		{123_456_789, 3_580_000_000, 441_975_304},
		{-123_456_789, 3_580_000_000, -441_975_304},
		{math.MaxInt64, 500_000_000, 4_611_686_018_427_387_903},
		{math.MinInt64, 500_000_000, -4_611_686_018_427_387_904},
		{math.MaxInt64, 1_000_000_000, math.MaxInt64},
		{math.MinInt64, 1_000_000_000, math.MinInt64},
		{math.MaxInt64, 2_000_000_000, math.MaxInt64},
		{math.MinInt64, 2_000_000_000, math.MinInt64},
		{1, math.MaxUint64, 18_446_744_073},
		{math.MaxInt64, math.MaxUint64, math.MaxInt64},
		{math.MinInt64, math.MaxUint64, math.MinInt64},
	} {
		if got := runtime.ScaleNanosecondsFrequency(tc.ns, tc.frequency); got != tc.want {
			t.Errorf("scale(%d, %d) = %d, want %d", tc.ns, tc.frequency, got, tc.want)
		}
	}
}
