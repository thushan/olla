package logger

import "strings"

func stripAnsiCodes(s string) string {
	// matches \x1b[...m sequences, probably a better way to do this but this
	// seems to work for now, matt will not be happy i didn't use regex :P
	var b strings.Builder
	b.Grow(len(s))

	inEscape := false

	for i := 0; i < len(s); i++ {
		if !inEscape {
			if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
				inEscape = true
				i++ // skip the '['
				continue
			}
			b.WriteByte(s[i])
			continue
		}

		// We're in escape sequence; look for end token
		if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
			inEscape = false
		}
	}

	return b.String()
}
