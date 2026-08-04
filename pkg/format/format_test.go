package format

import (
	"testing"
	"time"
)

func TestDuration2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		// Sub-hour values
		{"30 seconds", 30 * time.Second, "30s"},
		{"5 minutes", 5 * time.Minute, "5m"},
		{"90 minutes", 90 * time.Minute, "1h 30m"},
		// Sub-day values
		{"1 hour", 1 * time.Hour, "1h 0m"},
		{"23 hours", 23 * time.Hour, "23h 0m"},
		// Exact multiples of 24 hours - these triggered the divide-by-zero panic.
		{"24 hours", 24 * time.Hour, "1d 0h"},
		{"48 hours", 48 * time.Hour, "2d 0h"},
		{"72 hours", 72 * time.Hour, "3d 0h"},
		// Non-multiples that produced wrong values before the fix.
		{"25 hours", 25 * time.Hour, "1d 1h"},
		{"31 hours", 31 * time.Hour, "1d 7h"},
		{"49 hours", 49 * time.Hour, "2d 1h"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Duration2(tc.d)
			if got != tc.want {
				t.Errorf("Duration2(%v) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}

func TestEndpointsUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		healthy int
		total   int
		want    string
	}{
		// Single-rune fast path, both sides safely single-digit.
		{"single digit both", 9, 9, "9/9"},
		// The fencepost: healthy==10 alone used to corrupt the healthy rune.
		{"healthy at 10", 10, 10, "10/10"},
		// total==10 alone used to corrupt the total rune - this is the exact
		// case observed live ("9/:") with a 10-endpoint fleet.
		{"total at ten, healthy single digit", 9, 10, "9/10"},
		// One side just over the fast-path boundary.
		{"total at eleven", 10, 11, "10/11"},
		{"both eleven", 11, 11, "11/11"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := EndpointsUp(tc.healthy, tc.total)
			if got != tc.want {
				t.Errorf("EndpointsUp(%d, %d) = %q, want %q", tc.healthy, tc.total, got, tc.want)
			}
		})
	}
}
