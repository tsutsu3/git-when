package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tsutsu3/git-when/internal/stats"
)

func TestDays(t *testing.T) {
	out := renderDays(t, sampleDays(), 6, Options{}) // The week starts on Sunday.

	header := strings.Fields(strings.ReplaceAll(strings.Split(out, "\n")[0], "│", " "))
	if got := strings.Join(header, " "); got != "Sun* Mon Tue Wed Thu Fri Sat*" {
		t.Errorf("header = %q, want Sun* .. Sat* with weekends marked", got)
	}

	// All days share one scale. Mon (peak) fills the bar and Sun is half.
	ten := panels(t, out, "10")
	if len(ten) != 7 {
		t.Fatalf("hour 10 has %d panels, want 7: %q", len(ten), ten)
	}
	if n := strings.Count(ten[1], "█"); n != dayBarWidth {
		t.Errorf("Mon 10 bar = %d blocks, want %d: %q", n, dayBarWidth, ten[1])
	}
	if n := strings.Count(ten[0], "█"); n != dayBarWidth/2 {
		t.Errorf("Sun 10 bar = %d blocks, want %d: %q", n, dayBarWidth/2, ten[0])
	}
	if !strings.HasSuffix(ten[1], " 1000") || !strings.HasSuffix(ten[0], "  500") {
		t.Errorf("hour 10 counts = %q, %q", ten[1], ten[0])
	}
	// A bar at 1/1000 of the peak is still drawn.
	if three := panels(t, out, "03"); !strings.ContainsAny(three[6], string(eighths)) {
		t.Errorf("Sat 03 bar is invisible: %q", three[6])
	}
	// Every panel has the same width, so separators line up across rows.
	for _, p := range panels(t, out, "00") {
		if Width(p) != dayBarWidth+1+len("1000") {
			t.Errorf("panel width = %d, want %d: %q", Width(p), dayBarWidth+5, p)
		}
	}

	if !strings.Contains(out, "all days share one scale (max 1000 at Mon 10); * = weekend") {
		t.Errorf("footer lacks the scale:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("colors without Color:\n%q", out)
	}
}

// TestDaysColor checks that Color does not change the bars.
// Night hours are not highlighted.
func TestDaysColor(t *testing.T) {
	plain := renderDays(t, sampleDays(), 0, Options{})
	if colored := renderDays(t, sampleDays(), 0, Options{Color: true}); colored != plain {
		t.Errorf("Color changes the days view:\n%q\nwant\n%q", colored, plain)
	}
}

func TestDaysEmpty(t *testing.T) {
	if out := renderDays(t, [7]stats.Day{}, 0, Options{}); out != "no commits\n" {
		t.Errorf("out = %q", out)
	}
}

func sampleDays() [7]stats.Day {
	var days [7]stats.Day
	for d := range days {
		days[d] = stats.Day{Weekday: d, PeakHour: -1}
	}
	days[0].Hours[10] = 1000 // Mon 10 (peak)
	days[6].Hours[10] = 500  // Sun 10
	days[5].Hours[3] = 1     // Sat 03
	for d := range days {
		for _, n := range days[d].Hours {
			days[d].Commits += n
		}
	}
	return days
}

func renderDays(t *testing.T, days [7]stats.Day, weekStart int, o Options) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Days(&buf, days, weekStart, stats.DefaultWeekend(), o); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// panels splits the row of hour h into one panel per weekday.
func panels(t *testing.T, out, hour string) []string {
	t.Helper()
	for l := range strings.SplitSeq(out, "\n") {
		if rest, ok := strings.CutPrefix(l, hour+"  "); ok {
			return strings.Split(rest, " │ ")
		}
	}
	t.Fatalf("no row %s in:\n%s", hour, out)
	return nil
}
