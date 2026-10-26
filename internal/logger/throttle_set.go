package logger

import "sync"

// ThrottleSet collects the throttles one owner created so it can settle them
// together at shutdown. Each Throttle only writes a summary from inside Allow
// (a later rejection through that same throttle) or Flush, so without a Flush
// the tail of a flood that stops just before shutdown is never reported.
//
// Both methods are safe on a nil set, which lets tests and library callers that
// have no shutdown path pass nil.
type ThrottleSet struct {
	throttles []*Throttle
	mu        sync.Mutex
}

// Track registers t and returns it, so construction and registration read as one step.
func (s *ThrottleSet) Track(t *Throttle) *Throttle {
	if s == nil || t == nil {
		return t
	}
	s.mu.Lock()
	s.throttles = append(s.throttles, t)
	s.mu.Unlock()
	return t
}

// Flush reports every tracked throttle's pending summaries.
func (s *ThrottleSet) Flush() {
	if s == nil {
		return
	}
	s.mu.Lock()
	ts := append([]*Throttle(nil), s.throttles...)
	s.mu.Unlock()
	for _, t := range ts {
		t.Flush()
	}
}
