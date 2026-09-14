package render

import (
	"strings"
	"unicode"

	"golang.org/x/text/width"
)

// Width returns the display width of s in a terminal.
// East Asian Wide and Fullwidth characters count as 2.
// Characters without width, such as combining marks, count as 0.
// fmt's %-*s pads by rune count, so tables break with Japanese text. Use Width for alignment.
func Width(s string) int {
	n := 0
	for _, r := range s {
		n += runeWidth(r)
	}
	return n
}

// PadRight adds spaces on the right until the display width is w.
// Longer strings are shortened with Truncate.
func PadRight(s string, w int) string {
	s = Truncate(s, w)
	return s + strings.Repeat(" ", max(0, w-Width(s)))
}

// PadLeft adds spaces on the left until the display width is w.
// Longer strings are shortened with Truncate.
func PadLeft(s string, w int) string {
	s = Truncate(s, w)
	return strings.Repeat(" ", max(0, w-Width(s))) + s
}

// Truncate shortens s to display width w, ending with "…", when s is wider than w.
func Truncate(s string, w int) string {
	if Width(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := runeWidth(r)
		if used+rw > w-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	b.WriteString("…")
	return b.String()
}

func runeWidth(r rune) int {
	if unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf) {
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	default:
		return 1
	}
}
