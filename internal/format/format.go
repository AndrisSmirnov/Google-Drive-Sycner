// Package format renders sizes and numbers the way the interface shows them.
package format

import (
	"strconv"
	"strings"
)

const (
	Kb int64 = 1 << 10
	Mb int64 = 1 << 20
	Gb int64 = 1 << 30
	Tb int64 = 1 << 40
)

// Size renders a byte count as 3Kb, 199Mb, 1.1Gb, 12Gb.
func Size(n int64) string {
	switch {
	case n >= Tb:
		return scaled(n, Tb) + "Tb"
	case n >= Gb:
		return scaled(n, Gb) + "Gb"
	case n >= Mb:
		return scaled(n, Mb) + "Mb"
	case n >= Kb:
		return scaled(n, Kb) + "Kb"
	default:
		return strconv.FormatInt(n, 10) + "b"
	}
}

// scaled keeps one decimal below 10 units and none above, dropping a trailing ".0".
func scaled(n, unit int64) string {
	v := float64(n) / float64(unit)
	if v >= 10 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strings.TrimSuffix(strconv.FormatFloat(v, 'f', 1, 64), ".0")
}

// Group splits the digits of n into thousands with sep: 18207 -> "18 207".
func Group(n int, sep string) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString(sep)
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
