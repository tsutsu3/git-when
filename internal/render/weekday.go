package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tsutsu3/git-when/internal/axis"
	"github.com/tsutsu3/git-when/internal/stats"
)

// Weekdays draws commits per weekday as bars on one shared scale, one day per line.
// Lines start at weekStart (0=Mon..6=Sun). Weekend days are marked.
func Weekdays(
	w io.Writer, days [7]stats.Day, weekStart int, weekend stats.Weekend, _ Options,
) error {
	total, peak := 0, 0
	for _, d := range days {
		total += d.Commits
		peak = max(peak, d.Commits)
	}
	if total == 0 {
		_, err := io.WriteString(w, "no commits\n")
		return err
	}

	const none = "-"
	countWidth := max(len("commits"), len(strconv.Itoa(peak)))

	var b strings.Builder
	fmt.Fprintf(&b, "day  %s  %*s  %6s  %9s  %6s\n",
		PadRight("", weekBarWidth), countWidth, "commits", "share", "peak hour", "night")

	start := ((weekStart % len(days)) + len(days)) % len(days)
	for i := range len(days) {
		d := days[(start+i)%len(days)]
		peakHour, night := none, none
		if d.Commits > 0 {
			peakHour = fmt.Sprintf("%02d", d.PeakHour)
			night = fmt.Sprintf("%.1f%%", d.NightShare*100)
		}
		mark := ""
		if weekend[d.Weekday] {
			mark = "  weekend"
		}
		fmt.Fprintf(&b, "%s  %s  %*d  %6s  %9s  %6s%s\n",
			axis.WeekdayName(d.Weekday), bar(d.Commits, peak, weekBarWidth),
			countWidth, d.Commits, percent(d.Commits, total), peakHour, night, mark)
	}

	// An even split gives each day 1/7 of the commits.
	weekendDays := weekend.Days()
	weekendTotal := 0
	for _, d := range days {
		if weekend[d.Weekday] {
			weekendTotal += d.Commits
		}
	}
	fmt.Fprintf(&b, "\nall days share one scale (max %d); an even split is %.1f%% per day\n",
		peak, 100.0/float64(len(days)))
	if weekendDays > 0 && weekendDays < len(days) {
		fmt.Fprintf(&b, "average per weekday %.1f, per weekend day %.1f\n",
			float64(total-weekendTotal)/float64(len(days)-weekendDays),
			float64(weekendTotal)/float64(weekendDays))
	}

	_, err := io.WriteString(w, b.String())
	return err
}
