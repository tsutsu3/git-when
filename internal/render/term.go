// Package render draws aggregated commits in forms that people read.
//
// It writes terminal views, CSV, SVG, and the single-file HTML report.
package render

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/tsutsu3/git-when/internal/axis"
	"github.com/tsutsu3/git-when/internal/stats"
)

const ansiReset = "\x1b[0m"

const (
	minCellWidth  = 2
	maxCellWidth  = 8
	maxLabelWidth = 32
	weekBarWidth  = 30
	hourGuide     = 6 // Hour axes get a separator every hourGuide hours.
)

// shades[level] is the character for a shading level without colors.
// Zero commits are blank, so the pattern stands out.
//
// Shade blocks fill the whole cell. Bars of different heights (▁▂▃) would turn each row
// into a small bar chart, and small values would sink to the bottom of the cell.
// Levels is derived from the number of characters here.
var shades = [...]rune{' ', '░', '▒', '▓', '█'}

// eighths[n-1] is a horizontal bar n/8 of a character long.
var eighths = []rune("▏▎▍▌▋▊▉")

// Options configures rendering.
type Options struct {
	// Color enables ANSI colors in terminal output.
	Color bool
	// Scale maps commit counts to shading levels in grids.
	Scale Scale
	// Theme is the terminal background. It decides the direction of the color ramps.
	Theme Theme
	// MinRowTotal hides grid rows whose total is below this value. Zero shows all rows.
	MinRowTotal int
	// SVGTheme is the SVG color scheme.
	SVGTheme SVGTheme
}

// shownRows holds the rows of a grid to draw.
// The peak and its position are computed from these rows only.
type shownRows struct {
	rows         []int // Indexes of the rows to draw.
	totals       []int // Totals of all rows, including hidden rows.
	hidden       int   // Number of hidden rows.
	maxTotal     int   // Largest total among the drawn rows.
	peak         int
	peakY, peakX int
}

// visibleRows selects the rows whose total is at least minTotal.
// With many authors or repositories, rows with almost no commits would fill the view.
//
// The peak is computed again from the drawn rows.
// A large cell in a hidden row would make all drawn rows look pale.
// The scan is row-first like axis.Fold, and the first of equal values wins.
func visibleRows(g axis.Grid, minTotal int) shownRows {
	s := shownRows{totals: g.RowTotals()}
	for iy, total := range s.totals {
		if total < minTotal {
			s.hidden++
			continue
		}
		s.rows = append(s.rows, iy)
		s.maxTotal = max(s.maxTotal, total)
		for ix, n := range g.Cells[iy] {
			if n > s.peak {
				s.peak, s.peakY, s.peakX = n, iy, ix
			}
		}
	}
	return s
}

// Heatmap draws g with shading.
// With colors, cells get a background color. Without colors, cells are filled with shade characters.
// Every grid, such as wday/hour or year/hour, is drawn the same way with different axes.
func Heatmap(w io.Writer, g axis.Grid, o Options) error {
	if g.Total == 0 {
		_, err := io.WriteString(w, "no commits\n")
		return err
	}

	v := visibleRows(g, o.MinRowTotal)
	if len(v.rows) == 0 {
		_, err := fmt.Fprintf(w, "no %s with at least %d commits (%d hidden)\n",
			rowsName(g.Y), o.MinRowTotal, v.hidden)
		return err
	}
	rows, totals, peak := v.rows, v.totals, v.peak
	shownLabels := make([]string, len(rows))
	for i, iy := range rows {
		shownLabels[i] = g.Y.Label(iy)
	}

	cellWidth := min(max(maxWidth(labels(g.X)), minCellWidth), maxCellWidth)
	labelWidth := min(maxWidth(shownLabels), maxLabelWidth)
	totalWidth := len(strconv.Itoa(v.maxTotal))

	// Blank cells are hard to follow across a row, so hour axes get a separator every six hours.
	sep := func(ix int) string {
		if g.X.Kind == axis.Hour && ix > 0 && ix%hourGuide == 0 {
			return "│"
		}
		return " "
	}

	var b strings.Builder
	header := strings.Repeat(" ", labelWidth)
	for ix := range g.X.Len() {
		header += sep(ix) + PadRight(g.X.Label(ix), cellWidth)
	}
	b.WriteString(strings.TrimRight(header, " ") + "\n")

	for _, iy := range rows {
		b.WriteString(PadRight(g.Y.Label(iy), labelWidth))
		for ix, n := range g.Cells[iy] {
			level := o.Scale.Level(n, peak)
			b.WriteString(sep(ix) + heatCell(level, cellWidth, o))
		}
		fmt.Fprintf(&b, "  %*d\n", totalWidth, totals[iy])
	}

	// A shaded chart without a legend is easy to misread.
	// Always write the scale, the maximum, and the lower bound of each level.
	steps := o.Scale.Steps(peak)
	fmt.Fprintf(&b, "\n%s scale, max %d at %s (blank = 0):",
		o.Scale, peak, peakLabel(g, v.peakY, v.peakX))
	for _, s := range steps {
		fmt.Fprintf(&b, "  %s %d+", heatCell(s.Level, minCellWidth, o), s.Min)
	}
	b.WriteString("\n")
	if v.hidden > 0 {
		fmt.Fprintf(&b, "%d of %d %s hidden: fewer than %d commits\n",
			v.hidden, len(totals), rowsName(g.Y), o.MinRowTotal)
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// Week draws hourly commits on weekdays and weekends as bars side by side.
//
// Both sides always share one scale.
// With separate scales, a quiet weekend would stretch as far as weekdays.
// That suggests the opposite conclusion.
func Week(w io.Writer, wk stats.Week, _ Options) error {
	total := wk.Total()
	if total == 0 {
		_, err := io.WriteString(w, "no commits\n")
		return err
	}

	peak := max(slices.Max(wk.OnWeekdays[:]), slices.Max(wk.OnWeekend[:]))
	countWidth := len(strconv.Itoa(peak))
	sideWidth := weekBarWidth + 1 + countWidth

	var b strings.Builder
	days := wk.Weekend.Days()
	left := fmt.Sprintf("weekdays (%d days)", len(wk.Weekend)-days)
	right := fmt.Sprintf("weekend: %s (%d days)", strings.Join(wk.Weekend.Names(), ", "), days)
	fmt.Fprintf(&b, "    %s  │  %s\n", PadRight(left, sideWidth), right)

	for h := range len(wk.OnWeekdays) {
		l := fmt.Sprintf(
			"%s %*d",
			bar(wk.OnWeekdays[h], peak, weekBarWidth),
			countWidth,
			wk.OnWeekdays[h],
		)
		r := fmt.Sprintf(
			"%s %*d",
			bar(wk.OnWeekend[h], peak, weekBarWidth),
			countWidth,
			wk.OnWeekend[h],
		)
		fmt.Fprintf(&b, "%02d  %s  │  %s\n", h, l, r)
	}

	fmt.Fprintf(&b, "\nboth sides share one scale (max %d)\n", peak)
	fmt.Fprintf(&b, "weekdays %d (%s)  weekend %d (%s)\n",
		wk.WeekdayTotal(), percent(wk.WeekdayTotal(), total),
		wk.WeekendTotal(), percent(wk.WeekendTotal(), total))
	if idx, ok := wk.WeekendIndex(); ok {
		fmt.Fprintf(
			&b,
			"weekend index %.2f  (weekend share / even share %.1f%%; 1.00 = as busy as weekdays)\n",
			idx,
			wk.EvenShare()*100,
		)
	} else {
		b.WriteString("weekend index -\n")
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// Summary draws a table with one summary row per repository.
func Summary(
	w io.Writer,
	rows []stats.ProjectSummary,
	total stats.ProjectSummary,
	_ Options,
) error {
	header := []string{
		"project", "commits", "share", "authors", "peak hour", "peak day", "night", "weekend idx",
	}
	alignRight := []bool{false, true, true, true, true, false, true, true}

	body := make([][]string, len(rows))
	for i, r := range rows {
		body[i] = summaryCells(r, total.Commits)
	}
	footer := summaryCells(total, total.Commits)

	widths := make([]int, len(header))
	for _, cells := range append([][]string{header, footer}, body...) {
		for i, c := range cells {
			widths[i] = max(widths[i], Width(c))
		}
	}
	widths[0] = min(widths[0], maxLabelWidth)

	rule := make([]string, len(widths))
	for i, wd := range widths {
		rule[i] = strings.Repeat("-", wd)
	}

	var b strings.Builder
	writeRow(&b, header, widths, alignRight)
	writeRow(&b, rule, widths, alignRight)
	for _, cells := range body {
		writeRow(&b, cells, widths, alignRight)
	}
	writeRow(&b, rule, widths, alignRight)
	writeRow(&b, footer, widths, alignRight)

	_, err := io.WriteString(w, b.String())
	return err
}

// peakLabel names the position of the peak cell, such as "Mon 09".
// A grid without a y axis uses only the x label.
func peakLabel(g axis.Grid, iy, ix int) string {
	at := g.X.Label(ix)
	if y := g.Y.Label(iy); y != "" {
		at = y + " " + at
	}
	return at
}

// rowsName names the rows of a grid with a on the y axis, such as "author rows".
func rowsName(a axis.Axis) string {
	if a.Name() == "" {
		return "rows"
	}
	return a.Name() + " rows"
}

// bar draws v as a bar up to width characters long, relative to peak.
// It grows in steps of 1/8 character.
// Any value above zero draws at least ▏, so it differs from no commits.
func bar(v, peak, width int) string {
	if v <= 0 || peak <= 0 {
		return strings.Repeat(" ", width)
	}
	n := v * width * 8 / peak
	if n == 0 {
		n = 1
	}
	s := strings.Repeat("█", n/8)
	if rem := n % 8; rem > 0 {
		s += string(eighths[rem-1])
	}
	return PadRight(s, width)
}

func summaryCells(s stats.ProjectSummary, all int) []string {
	const none = "-"
	peakHour, peakDay, night := none, none, none
	if s.Commits > 0 {
		peakHour = fmt.Sprintf("%02d", s.PeakHour)
		peakDay = axis.WeekdayName(s.PeakWeekday)
		night = fmt.Sprintf("%.1f%%", s.NightShare*100)
	}
	weekend := none
	if idx, ok := s.Week.WeekendIndex(); ok {
		weekend = fmt.Sprintf("%.2f", idx)
	}
	return []string{
		s.Name, strconv.Itoa(s.Commits), percent(s.Commits, all), strconv.Itoa(s.Authors),
		peakHour, peakDay, night, weekend,
	}
}

func writeRow(b *strings.Builder, cells []string, widths []int, alignRight []bool) {
	parts := make([]string, len(cells))
	for i, c := range cells {
		if alignRight[i] {
			parts[i] = PadLeft(c, widths[i])
		} else {
			parts[i] = PadRight(c, widths[i])
		}
	}
	b.WriteString(strings.TrimRight(strings.Join(parts, "  "), " ") + "\n")
}

func percent(n, total int) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", float64(n)*100/float64(total))
}

func labels(a axis.Axis) []string {
	out := make([]string, a.Len())
	for i := range out {
		out[i] = a.Label(i)
	}
	return out
}

func maxWidth(ss []string) int {
	n := 0
	for _, s := range ss {
		n = max(n, Width(s))
	}
	return n
}
