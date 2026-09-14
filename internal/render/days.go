package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tsutsu3/git-when/internal/axis"
	"github.com/tsutsu3/git-when/internal/stats"
)

// dayBarWidth is the bar length for one weekday in Days.
// It is short so that seven columns fit in a terminal.
const dayBarWidth = 8

// Days draws hourly commits as bars in seven columns, one per weekday.
// It is the Week view split into seven days instead of weekdays and weekend.
//
// All seven columns share one scale.
// With a scale per column, a quiet day would fill its column and differences would disappear.
// Columns start at weekStart (0=Mon..6=Sun). Weekend days get a * in the heading.
func Days(
	w io.Writer, days [7]stats.Day, weekStart int, weekend stats.Weekend, _ Options,
) error {
	total, peak, peakDay, peakHour := 0, 0, 0, 0
	for _, d := range days {
		total += d.Commits
		for h, n := range d.Hours {
			if n > peak {
				peak, peakDay, peakHour = n, d.Weekday, h
			}
		}
	}
	if total == 0 {
		_, err := io.WriteString(w, "no commits\n")
		return err
	}

	countWidth := len(strconv.Itoa(peak))
	panelWidth := dayBarWidth + 1 + countWidth
	start := ((weekStart % len(days)) + len(days)) % len(days)
	order := make([]stats.Day, len(days))
	for i := range order {
		order[i] = days[(start+i)%len(days)]
	}

	var b strings.Builder
	headers := make([]string, len(order))
	for i, d := range order {
		name := axis.WeekdayName(d.Weekday)
		if weekend[d.Weekday] {
			name += "*"
		}
		headers[i] = PadRight(name, panelWidth)
	}
	b.WriteString(strings.TrimRight("    "+strings.Join(headers, " │ "), " ") + "\n")

	for h := range len(days[0].Hours) {
		panels := make([]string, len(order))
		for i, d := range order {
			panels[i] = fmt.Sprintf(
				"%s %*d",
				bar(d.Hours[h], peak, dayBarWidth),
				countWidth,
				d.Hours[h],
			)
		}
		fmt.Fprintf(&b, "%02d  %s\n", h, strings.Join(panels, " │ "))
	}

	fmt.Fprintf(&b, "\nall days share one scale (max %d at %s %02d); * = weekend\n",
		peak, axis.WeekdayName(peakDay), peakHour)

	_, err := io.WriteString(w, b.String())
	return err
}
