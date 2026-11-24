package git

import "strings"

// SanitizeLine removes control characters and terminal escape sequences from
// text that came from the repository (subjects, names, ref names), so it can
// never reach the terminal raw. Tabs become spaces; everything else that is a
// control character (C0, DEL, C1) is dropped, along with the rest of any
// ESC / CSI / OSC sequence it starts.
func SanitizeLine(s string) string {
	clean := true
	for _, r := range s {
		if r == '\t' || isControl(r) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case r == 0x1b || r == 0x9b || r == 0x9d:
			i = skipEscape(rs, i)
		case isControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0)
}

// skipEscape returns the index of the last rune of the escape sequence that
// starts at rs[i] (ESC, CSI 0x9b or OSC 0x9d).
func skipEscape(rs []rune, i int) int {
	kind := rs[i]
	if kind == 0x1b {
		if i+1 >= len(rs) {
			return i
		}
		switch rs[i+1] {
		case '[':
			kind, i = 0x9b, i+1
		case ']', 'P', '_', '^', 'X':
			kind, i = 0x9d, i+1
		default:
			return i + 1 // two-character escape (ESC c, ESC 7, …)
		}
	}
	switch kind {
	case 0x9b: // CSI: parameters and intermediates up to a final byte @–~
		for j := i + 1; j < len(rs); j++ {
			switch {
			case rs[j] >= 0x40 && rs[j] <= 0x7e:
				return j
			case rs[j] < 0x20 || rs[j] > 0x7e:
				return j - 1 // malformed: drop what was read, keep the rest
			}
		}
		return len(rs) - 1
	default: // OSC / DCS / APC / PM / SOS: up to BEL or ST (ESC \ or 0x9c)
		for j := i + 1; j < len(rs); j++ {
			switch {
			case rs[j] == 0x07 || rs[j] == 0x9c:
				return j
			case rs[j] == 0x1b && j+1 < len(rs) && rs[j+1] == '\\':
				return j + 1
			}
		}
		return len(rs) - 1
	}
}
