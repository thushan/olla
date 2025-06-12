package util

import (
	"time"
)

// ParseTime attempts to parse a time string in RFC3339 or RFC3339Nano format for LLM Frontend Profiles
func ParseTime(timeStr string) *time.Time {
	// Try RFC3339 format first (standard ISO format)
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return &t
	}
	// Try RFC3339Nano for higher precision
	if t, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
		return &t
	}
	return nil
}
