package profile

import (
	"time"
)

func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getFloat64(m map[string]interface{}, key string) (int64, bool) {
	if val, ok := m[key]; ok {
		if f, ok := val.(float64); ok {
			return int64(f), true
		}
	}
	return 0, false
}

func getStringArray(m map[string]interface{}, key string) []string {
	if val, ok := m[key]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if str, ok := item.(string); ok && str != "" {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return nil
}

func parseTime(m map[string]interface{}, key string) *time.Time {
	if val, ok := m[key]; ok {
		if timeStr, ok := val.(string); ok && timeStr != "" {
			// Try RFC3339 format first (standard ISO format)
			if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
				return &t
			}
			// Try RFC3339Nano for higher precision
			if t, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
				return &t
			}
		}
	}
	return nil
}
