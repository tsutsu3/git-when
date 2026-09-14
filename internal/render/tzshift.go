package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tsutsu3/git-when/internal/stats"
)

// FormatOffset formats a UTC offset in seconds, such as "+09:00".
func FormatOffset(sec int32) string {
	sign := '+'
	if sec < 0 {
		sign, sec = '-', -sec
	}
	return fmt.Sprintf("%c%02d:%02d", sign, sec/3600, sec%3600/60)
}

// TZShift draws a table of UTC offsets per year.
// The last column shows the most common offset of each year and its share.
// It adds "changed" when that offset differs from the year before.
// This shows the years of a move or a long trip.
func TZShift(w io.Writer, tz stats.TZShift, _ Options) error {
	if len(tz.Years) == 0 {
		_, err := io.WriteString(w, "no commits\n")
		return err
	}

	header := []string{"year"}
	alignRight := []bool{false}
	for _, off := range tz.Offsets {
		header = append(header, FormatOffset(off))
		alignRight = append(alignRight, true)
	}
	if tz.Other != nil {
		header = append(header, "other")
		alignRight = append(alignRight, true)
	}
	header = append(header, "total", "most common")
	alignRight = append(alignRight, true, false)

	rows := [][]string{header}
	var prev int32
	havePrev := false
	for i, year := range tz.Years {
		cells := []string{strconv.Itoa(year)}
		for _, n := range tz.Counts[i] {
			cells = append(cells, blankIfZero(n))
		}
		if tz.Other != nil {
			cells = append(cells, blankIfZero(tz.Other[i]))
		}
		main := ""
		if tz.MainCount[i] > 0 {
			main = fmt.Sprintf("%s %3.0f%%", FormatOffset(tz.Main[i]),
				float64(tz.MainCount[i])*100/float64(tz.Total[i]))
			if havePrev && tz.Main[i] != prev {
				main += "  changed"
			}
			prev, havePrev = tz.Main[i], true
		}
		rows = append(rows, append(cells, strconv.Itoa(tz.Total[i]), main))
	}

	widths := make([]int, len(header))
	for _, cells := range rows {
		for i, c := range cells {
			widths[i] = max(widths[i], Width(c))
		}
	}

	var b strings.Builder
	for _, cells := range rows {
		writeRow(&b, cells, widths, alignRight)
	}
	b.WriteString("\nUTC offsets as recorded by the authors")
	if tz.Other != nil {
		fmt.Fprintf(&b, "; offsets beyond the %d most used are in other", len(tz.Offsets))
	}
	b.WriteString("\n")

	_, err := io.WriteString(w, b.String())
	return err
}

func blankIfZero(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}
