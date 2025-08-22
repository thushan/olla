package pattern

import "strings"

// MatchesGlob checks if a string matches a glob pattern with * wildcard support
// This is the centralised pattern matching logic used throughout Olla
func MatchesGlob(s, pattern string) bool {
	// glob matching for * wildcard
	pattern = strings.ToLower(pattern)
	s = strings.ToLower(s)

	switch {
	case pattern == "*":
		return true
	case strings.Contains(pattern, "*"):
		// for patterns like "*llava*" or "llava*" or "*llava"
		switch {
		case strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*"):
			// *text* - contains
			core := strings.Trim(pattern, "*")
			return strings.Contains(s, core)
		case strings.HasPrefix(pattern, "*"):
			// *text - ends with
			suffix := strings.TrimPrefix(pattern, "*")
			return strings.HasSuffix(s, suffix)
		case strings.HasSuffix(pattern, "*"):
			// text* - starts with
			prefix := strings.TrimSuffix(pattern, "*")
			return strings.HasPrefix(s, prefix)
		default:
			// shouldn't happen with validation, but be safe
			return s == pattern
		}
	default:
		// this is an exact matcha
		return s == pattern
	}
}
