package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestThrottleSet_FlushReportsEveryTrackedThrottle(t *testing.T) {
	t.Parallel()

	var a, b summaryRecorder
	set := &ThrottleSet{}
	ta := set.Track(NewThrottle(ThrottleConfig{OnSummary: a.record}))
	tb := set.Track(NewThrottle(ThrottleConfig{OnSummary: b.record}))
	for range 4 {
		ta.Allow("k")
	}
	for range 3 {
		tb.Allow("k")
	}

	set.Flush()
	_, sa := a.total()
	_, sb := b.total()
	require.Equal(t, 3, sa)
	require.Equal(t, 2, sb)
}

func TestThrottleSet_NilIsSafe(t *testing.T) {
	t.Parallel()

	var set *ThrottleSet
	th := set.Track(NewThrottle(ThrottleConfig{}))
	require.NotNil(t, th)
	require.NotPanics(t, set.Flush)
	require.Nil(t, set.Track(nil))
}
