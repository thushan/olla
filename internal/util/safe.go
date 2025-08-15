package util

import "math"

// SafeInt64Diff subtracts u2 from u1, returning int64 if safe; otherwise returns 0
func SafeInt64Diff(u1, u2 uint64) int64 {
	if u1 < u2 {
		return 0 // avoid underflow
	}
	diff := u1 - u2
	if diff > math.MaxInt64 {
		return 0 // avoid overflow
	}
	return int64(diff)
}

func SafeUint64(value int64) uint64 {
	if value < 0 {
		return 0
	}
	return uint64(value)
}

func SafeInt32(value int64) int32 {
	if value < math.MinInt32 {
		return math.MinInt32
	}
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(value)
}
