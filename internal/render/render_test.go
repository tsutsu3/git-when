package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/tsutsu3/git-when/internal/model"
	"github.com/tsutsu3/git-when/internal/stats"
)

func TestWidth(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"日本語", 6},
		{"ｱｲｳ", 3},     // Half-width katakana.
		{"e\u0301", 1}, // A combining mark.
		{"🙂", 2},
		{"a日b", 4},
	} {
		if got := Width(c.in); got != c.want {
			t.Errorf("Width(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPadAndTruncate(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{PadRight("日本", 6), "日本  "},
		{PadLeft("日本", 6), "  日本"},
		{PadRight("abc", 2), "a…"},
		{Truncate("日本語テキスト", 7), "日本語…"},
		// A wide character does not fit in the last column, so a space fills it.
		{PadRight("日本語", 4), "日… "},
		{Truncate("abc", 3), "abc"},
		{Truncate("abc", 0), ""},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestScaleLevel(t *testing.T) {
	for _, s := range []Scale{Sqrt, Linear, Log} {
		for _, peak := range []int{1, 2, 7, 100, 5000} {
			if s.Level(0, peak) != 0 || s.Level(peak, peak) != Levels {
				t.Errorf(
					"%s peak %d: Level(0)=%d Level(peak)=%d",
					s,
					peak,
					s.Level(0, peak),
					s.Level(peak, peak),
				)
			}
			prev := 0
			for v := 1; v <= peak; v++ {
				l := s.Level(v, peak)
				if l < 1 || l < prev {
					t.Fatalf("%s peak %d: Level(%d) = %d after %d; want >= 1 and non-decreasing",
						s, peak, v, l, prev)
				}
				prev = l
			}
		}
	}
	// Square root shows small counts darker than linear.
	if Sqrt.Level(25, 100) <= Linear.Level(25, 100) {
		t.Errorf("Sqrt.Level(25,100)=%d, Linear=%d; want sqrt darker",
			Sqrt.Level(25, 100), Linear.Level(25, 100))
	}
}

func TestScaleSteps(t *testing.T) {
	for _, s := range []Scale{Sqrt, Linear, Log} {
		for _, peak := range []int{1, 3, 100, 5000} {
			steps := s.Steps(peak)
			if len(steps) == 0 || steps[len(steps)-1].Level != Levels {
				t.Errorf("%s peak %d: steps %+v must end at level %d", s, peak, steps, Levels)
			}
			for i, st := range steps {
				// Min is the smallest count in the level. One less falls to the level below.
				if s.Level(st.Min, peak) != st.Level ||
					(st.Min > 1 && s.Level(st.Min-1, peak) >= st.Level) {
					t.Errorf("%s peak %d: step %+v is not the lower bound", s, peak, st)
				}
				if i > 0 && st.Min <= steps[i-1].Min {
					t.Errorf("%s peak %d: steps not increasing: %+v", s, peak, steps)
				}
			}
		}
	}
}

func TestParseScale(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Scale
	}{
		{"sqrt", Sqrt},
		{"Linear", Linear},
		{" log ", Log}, // Surrounding spaces are ignored.
	} {
		if got, err := ParseScale(c.in); err != nil || got != c.want {
			t.Errorf("ParseScale(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	if _, err := ParseScale("cubic"); err == nil {
		t.Error("ParseScale(cubic) succeeded, want error")
	}
}

// TestWeekSameScale checks that both sides share one scale.
// Half as many weekend commits give a bar half as long.
func TestWeekSameScale(t *testing.T) {
	wk := stats.Week{Weekend: stats.DefaultWeekend()}
	wk.OnWeekdays[10] = 1000
	wk.OnWeekend[10] = 500
	wk.OnWeekend[3] = 1 // Still visible at 1/1000 of the peak.

	var buf bytes.Buffer
	if err := Week(&buf, wk, Options{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	sides := func(hour string) (left, right string) {
		t.Helper()
		for l := range strings.SplitSeq(out, "\n") {
			if strings.HasPrefix(l, hour+"  ") {
				left, right, _ = strings.Cut(l, "│")
				return left, right
			}
		}
		t.Fatalf("no row %s in:\n%s", hour, out)
		return "", ""
	}

	left, right := sides("10")
	if l, r := strings.Count(
		left,
		"█",
	), strings.Count(
		right,
		"█",
	); l != weekBarWidth ||
		r != weekBarWidth/2 {
		t.Errorf(
			"hour 10 bars = %d and %d blocks, want %d and %d",
			l,
			r,
			weekBarWidth,
			weekBarWidth/2,
		)
	}
	if _, right := sides("03"); !strings.Contains(right, "▏") {
		t.Errorf("hour 03 weekend bar is invisible: %q", right)
	}
	for _, want := range []string{
		"weekdays (5 days)", "weekend: Sat, Sun (2 days)", "one scale (max 1000)",
		"weekend index 1.17", // 501/1501 ÷ 2/7
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestSummaryAlignsColumns(t *testing.T) {
	rows, total := stats.Summarize(sample(t), stats.DefaultWeekend())
	var buf bytes.Buffer
	if err := Summary(&buf, rows, total, Options{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 6 { // Header, rule, two rows, rule, and total.
		t.Fatalf("got %d lines:\n%s", len(lines), out)
	}
	// The expected text does not use Width.
	// The project column is 18 wide, which is the width of the Japanese name.
	// Two separator spaces and the right-aligned commits column (7 wide) follow.
	for _, want := range []string{
		"project" + strings.Repeat(" ", 11) + "  commits",
		"日本語プロジェクト  " + "      2",
		"abc" + strings.Repeat(" ", 15) + "  " + "      2",
		"total" + strings.Repeat(" ", 13) + "  " + "      4",
	} {
		found := false
		for _, l := range lines {
			if strings.HasPrefix(l, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no line starts with %q:\n%s", want, out)
		}
	}
	for _, want := range []string{"日本語プロジェクト", "total", "peak hour", "100.0%"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
}

// sample has two commits on Tue 09 in the Japanese-named project.
// It has one commit on Sun 23 and one on Mon 14 in abc.
func sample(t *testing.T) *model.Aggregate {
	t.Helper()
	jst := time.FixedZone("JST", 9*3600)
	a := model.New()
	a.Authors = []model.Author{{Name: "Alice"}, {Name: "Bob"}}
	a.Projects = []model.Project{{Name: "日本語プロジェクト"}, {Name: "abc"}}
	for _, c := range []struct {
		author, project uint16
		at              time.Time
	}{
		{0, 0, time.Date(2024, time.March, 5, 9, 0, 0, 0, jst)},
		{0, 0, time.Date(2024, time.March, 5, 9, 30, 0, 0, jst)},
		{0, 1, time.Date(2024, time.March, 10, 23, 0, 0, 0, jst)},
		{1, 1, time.Date(2024, time.March, 4, 14, 0, 0, 0, jst)},
	} {
		if err := a.Add(c.author, c.project, c.at, 9*3600); err != nil {
			t.Fatal(err)
		}
	}
	return a
}
