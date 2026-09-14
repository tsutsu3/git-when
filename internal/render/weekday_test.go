package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tsutsu3/git-when/internal/stats"
)

func TestWeekdays(t *testing.T) {
	var days [7]stats.Day
	for d := range days {
		days[d] = stats.Day{Weekday: d, PeakHour: -1}
	}
	days[0] = stats.Day{Weekday: 0, Commits: 100, PeakHour: 21, NightShare: 0.25} // Mon
	days[5] = stats.Day{Weekday: 5, Commits: 50, PeakHour: 10}                    // Sat
	days[6] = stats.Day{Weekday: 6, Commits: 1, PeakHour: 3, NightShare: 1}       // Sun

	var buf bytes.Buffer
	if err := Weekdays(&buf, days, 6, stats.DefaultWeekend(), Options{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	lines := strings.Split(out, "\n")

	// The week starts on Sunday. Sun follows the header and Sat is last.
	var order []string
	for _, l := range lines[1:8] {
		order = append(order, l[:3])
	}
	if got := strings.Join(order, " "); got != "Sun Mon Tue Wed Thu Fri Sat" {
		t.Errorf("order = %q, want Sun..Sat", got)
	}

	row := func(day string) string {
		t.Helper()
		for _, l := range lines {
			if strings.HasPrefix(l, day+"  ") {
				return l
			}
		}
		t.Fatalf("no row %s:\n%s", day, out)
		return ""
	}
	// All days share one scale. Mon (peak) fills the bar and Sat is half.
	if n := strings.Count(row("Mon"), "█"); n != weekBarWidth {
		t.Errorf("Mon bar = %d blocks, want %d", n, weekBarWidth)
	}
	if n := strings.Count(row("Sat"), "█"); n != weekBarWidth/2 {
		t.Errorf("Sat bar = %d blocks, want %d", n, weekBarWidth/2)
	}
	// One commit out of a peak of 100 still draws a bar (2/8 of 30 characters is ▎).
	if !strings.ContainsAny(row("Sun"), string(eighths)) {
		t.Errorf("Sun bar is invisible: %q", row("Sun"))
	}

	for _, c := range []struct{ day, want string }{
		{"Mon", "    100   66.2%         21   25.0%"},
		{"Sat", "     50   33.1%         10    0.0%  weekend"},
		{"Sun", "      1    0.7%         03  100.0%  weekend"},
		{"Tue", "      0    0.0%          -       -"},
	} {
		if !strings.HasSuffix(row(c.day), c.want) {
			t.Errorf("%s row = %q, want suffix %q", c.day, row(c.day), c.want)
		}
	}
	if strings.Contains(row("Fri"), "weekend") {
		t.Errorf("Fri is marked as weekend: %q", row("Fri"))
	}
	// Weekdays average 100 / 5 days and weekend days average 51 / 2 days.
	if want := "average per weekday 20.0, per weekend day 25.5"; !strings.Contains(out, want) {
		t.Errorf("output lacks %q:\n%s", want, out)
	}
}

func TestWeekdaysEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := Weekdays(&buf, [7]stats.Day{}, 0, stats.DefaultWeekend(), Options{}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "no commits\n" {
		t.Errorf("out = %q", buf.String())
	}
}
