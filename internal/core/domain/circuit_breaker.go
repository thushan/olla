package domain

import "time"

type CircuitBreakerState struct {
	State                string     `json:"state"`
	LastTripTimestamp    *time.Time `json:"last_trip_ts,omitempty"`
	ConsecutiveFailures  int64      `json:"consecutive_failures"`
	CooldownRemainingSec int        `json:"cooldown_remaining_s"`
}
