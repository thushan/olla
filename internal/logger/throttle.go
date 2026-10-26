package logger

import (
	"strings"
	"sync"
	"time"
)

const (
	// DefaultThrottleWindow is how long a key stays quiet after its first event
	// is logged in full.
	DefaultThrottleWindow = 10 * time.Second

	// DefaultThrottleMaxKeys caps tracked keys. A storm of distinct client IPs
	// (spoofed or genuine) then costs at most this many full lines per window,
	// plus one shared summary, instead of one line each.
	DefaultThrottleMaxKeys = 256

	// MaxThrottleKeyLen bounds a stored key. Behind trusted proxies the key can be the raw
	// first X-Forwarded-For value, which an attacker controls up to the header size; the
	// table holds MaxKeys of them and each is echoed in a summary line.
	MaxThrottleKeyLen = 64

	// OverflowKey labels the shared bucket that absorbs keys arriving once the
	// cap is reached.
	OverflowKey = "other"
)

// ThrottleConfig configures a Throttle. Zero values take the defaults.
type ThrottleConfig struct {
	// OnSummary receives the count of events suppressed for a key. It is called
	// outside the throttle's lock, so it may log freely. Nil drops summaries.
	OnSummary func(key string, suppressed int, window time.Duration)
	// Now is the clock; tests inject a fake. Nil means time.Now.
	Now     func() time.Time
	Window  time.Duration
	MaxKeys int
}

type throttleEntry struct {
	start      time.Time
	suppressed int
}

type throttleSummary struct {
	key        string
	suppressed int
}

// Throttle bounds how often an attacker-triggerable rejection reaches the log.
// The first event per key per window is allowed through (the caller logs it in
// full); the rest are counted, and the count is reported once through
// OnSummary when the window rolls over or the key is evicted. Metrics counters
// stay the source of truth: the throttle only decides what is written.
//
// Memory is O(MaxKeys) whatever the key cardinality: once the cap is reached,
// new keys share a single overflow bucket. It starts no goroutines, so there is
// nothing to leak; summaries for a flood that has stopped are written by the
// next event for the key, by the sweep that runs inside Allow once a window has
// lapsed, or by Flush. Allow is only called on a rejection, so a sweep needs another
// rejection through this same throttle: with none, nothing is written until the next
// event or Flush, which is why owners flush their throttles when they shut down.
type Throttle struct {
	lastSweep time.Time
	now       func() time.Time
	onSummary func(key string, suppressed int, window time.Duration)
	entries   map[string]*throttleEntry
	overflow  throttleEntry
	window    time.Duration
	maxKeys   int
	mu        sync.Mutex
}

func NewThrottle(cfg ThrottleConfig) *Throttle {
	if cfg.Window <= 0 {
		cfg.Window = DefaultThrottleWindow
	}
	if cfg.MaxKeys <= 0 {
		cfg.MaxKeys = DefaultThrottleMaxKeys
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Throttle{
		now:       cfg.Now,
		onSummary: cfg.OnSummary,
		entries:   make(map[string]*throttleEntry, min(cfg.MaxKeys, 64)),
		window:    cfg.Window,
		maxKeys:   cfg.MaxKeys,
	}
}

// Allow reports whether the caller should log this event in full.
func (t *Throttle) Allow(key string) bool {
	now := t.now()
	if len(key) > MaxThrottleKeyLen {
		key = strings.ToValidUTF8(key[:MaxThrottleKeyLen], "")
	}

	var (
		allow     bool
		summaries []throttleSummary
		own       throttleSummary
	)

	t.mu.Lock()
	// Settle lapsed windows at most once per window, whoever is calling. Without this a
	// flood that stops leaves its tail count unreported until its own key returns, which
	// for a one-off attacker is never.
	if now.Sub(t.lastSweep) >= t.window {
		t.lastSweep = now
		summaries = t.sweepLocked(now)
	}
	e, ok := t.entries[key]
	switch {
	case ok:
		allow, own.suppressed = t.touch(e, now)
		own.key = key
	case len(t.entries) < t.maxKeys:
		// Clone: the key is often a substring of a much larger request value (a model name),
		// and keeping it would pin that whole allocation for every tracked key.
		t.entries[strings.Clone(key)] = &throttleEntry{start: now}
		allow = true
	default:
		// full: the sweep above already reclaimed what had lapsed
		if t.overflow.start.IsZero() {
			t.overflow.start = now
			allow = true
			break
		}
		allow, own.suppressed = t.touch(&t.overflow, now)
		own.key = OverflowKey
	}
	t.mu.Unlock()

	if own.suppressed > 0 {
		t.report(own.key, own.suppressed)
	}
	for _, s := range summaries {
		t.report(s.key, s.suppressed)
	}
	return allow
}

// touch registers an event on an existing entry. It returns whether the event
// opens a new window (and so should be logged) and, when it does, the number of
// events suppressed in the window that just ended.
func (t *Throttle) touch(e *throttleEntry, now time.Time) (allow bool, prior int) {
	if now.Sub(e.start) >= t.window {
		prior = e.suppressed
		e.start = now
		e.suppressed = 0
		return true, prior
	}
	e.suppressed++
	return false, 0
}

// sweepLocked drops entries whose window has lapsed and returns their pending
// summaries.
func (t *Throttle) sweepLocked(now time.Time) []throttleSummary {
	var out []throttleSummary
	for k, e := range t.entries {
		if now.Sub(e.start) < t.window {
			continue
		}
		if e.suppressed > 0 {
			out = append(out, throttleSummary{key: k, suppressed: e.suppressed})
		}
		delete(t.entries, k)
	}
	if !t.overflow.start.IsZero() && now.Sub(t.overflow.start) >= t.window {
		if t.overflow.suppressed > 0 {
			out = append(out, throttleSummary{key: OverflowKey, suppressed: t.overflow.suppressed})
		}
		t.overflow = throttleEntry{}
	}
	return out
}

// Flush reports every pending summary now. Call it when the owner shuts down so
// the tail of a flood is not lost.
func (t *Throttle) Flush() {
	t.mu.Lock()
	var out []throttleSummary
	for k, e := range t.entries {
		if e.suppressed > 0 {
			out = append(out, throttleSummary{key: k, suppressed: e.suppressed})
			e.suppressed = 0
		}
	}
	if t.overflow.suppressed > 0 {
		out = append(out, throttleSummary{key: OverflowKey, suppressed: t.overflow.suppressed})
		t.overflow.suppressed = 0
	}
	t.mu.Unlock()

	for _, s := range out {
		t.report(s.key, s.suppressed)
	}
}

// Len returns the number of tracked keys, excluding the overflow bucket.
func (t *Throttle) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}

func (t *Throttle) report(key string, suppressed int) {
	if t.onSummary != nil {
		t.onSummary(key, suppressed, t.window)
	}
}
