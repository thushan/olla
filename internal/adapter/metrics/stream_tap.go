package metrics

import (
	"bytes"
	"sync/atomic"
	"time"
)

// StreamTap implements io.Writer and is used with io.TeeReader to passively
// observe streaming data without blocking or copying. It measures the real
// Time to First Token (TTFT) from the SSE stream.
//
// Zero allocation on the hot path after construction — only records timestamps
// and counts bytes.
type StreamTap struct {
	startTime    time.Time
	firstTokenAt time.Time
	totalBytes   atomic.Int64
	hasFirstData atomic.Int32 // 0 = no data yet, 1 = first byte seen, 2 = first SSE data seen
}

// sseDataPrefix is the SSE data line prefix we look for to detect first real token
var sseDataPrefix = []byte("data: ")

// NewStreamTap creates a new StreamTap that measures timing from the given start time.
func NewStreamTap(startTime time.Time) *StreamTap {
	return &StreamTap{
		startTime: startTime,
	}
}

// Write implements io.Writer. Called by TeeReader for every chunk read from upstream.
// Must be non-blocking and infallible — we never want to slow down the response stream.
func (t *StreamTap) Write(p []byte) (n int, err error) {
	now := time.Now()
	t.totalBytes.Add(int64(len(p)))

	// Record first byte timestamp
	if t.hasFirstData.CompareAndSwap(0, 1) {
		t.firstTokenAt = now
	}

	// Detect first SSE data line (contains actual token content)
	// This gives a more accurate TTFT than just first byte which may be headers
	if t.hasFirstData.Load() < 2 && bytes.Contains(p, sseDataPrefix) {
		t.hasFirstData.Store(2)
		t.firstTokenAt = now
	}

	return len(p), nil
}

// FirstTokenTime returns the timestamp of the first SSE data received.
// Returns zero time if no data has been received.
func (t *StreamTap) FirstTokenTime() time.Time {
	return t.firstTokenAt
}

// TTFT returns the Time to First Token in milliseconds.
// Returns 0 if no token data has been received.
func (t *StreamTap) TTFT() int64 {
	if t.firstTokenAt.IsZero() {
		return 0
	}
	return t.firstTokenAt.Sub(t.startTime).Milliseconds()
}

// TotalBytes returns the total bytes observed.
func (t *StreamTap) TotalBytes() int64 {
	return t.totalBytes.Load()
}

// HasReceivedData returns true if any data has been written.
func (t *StreamTap) HasReceivedData() bool {
	return t.hasFirstData.Load() > 0
}
