package logger

import "time"

// NewRejectThrottle samples an attacker-triggerable per-request rejection WARN. The caller
// asks Allow(key) and logs the first rejection per key per window in full; the rest are
// counted and reported as one "<msg>, repeats suppressed" line carrying the key under keyAttr
// (client_ip for per-client rejections). layer names the throttle that wrote the summary: several
// layers share a message ("Request rejected" is written by both the size limit and the proxy
// handler) and run separate windows, so without it two summary lines read as one. now is nil
// outside tests.
func NewRejectThrottle(log StyledLogger, msg, layer, keyAttr string, now func() time.Time) *Throttle {
	return NewThrottle(ThrottleConfig{
		Now: now,
		OnSummary: func(key string, suppressed int, window time.Duration) {
			log.Warn(msg+", repeats suppressed",
				"layer", layer,
				keyAttr, key,
				"suppressed", suppressed,
				"window", window)
		},
	})
}
