package render

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tsutsu3/git-when/internal/axis"
	"github.com/tsutsu3/git-when/internal/model"
)

func TestHeatmap(t *testing.T) {
	g := axis.Fold(sample(t), axis.Spec{Y: axis.Weekday, X: axis.Hour}, 0, nil)
	out := renderHeatmap(t, g, Options{Scale: Sqrt})
	lines := strings.Split(out, "\n")

	// A separator appears every six hours.
	if want := "    00 01 02 03 04 05│06"; !strings.HasPrefix(lines[0], want) {
		t.Errorf("header = %q, want prefix %q", lines[0], want)
	}
	if want := "17│18 19 20 21 22 23"; !strings.HasSuffix(lines[0], want) {
		t.Errorf("header = %q, want suffix %q", lines[0], want)
	}

	row := func(label string) []rune {
		t.Helper()
		for _, l := range lines {
			if strings.HasPrefix(l, label+" ") {
				return []rune(l)
			}
		}
		t.Fatalf("no row %q in:\n%s", label, out)
		return nil
	}
	// After "Mon", each hour is one separator character and two cell characters.
	cell := func(r []rune, hour int) string {
		start := len("Mon") + 1 + hour*3
		return string(r[start : start+2])
	}
	for _, c := range []struct {
		day  string
		hour int
		want string
	}{
		{"Tue", 9, "██"},  // The peak (2 commits).
		{"Sun", 23, "░░"}, // One commit is always the lightest level.
		{"Mon", 14, "░░"},
		{"Tue", 10, "  "}, // Zero is blank.
		{"Wed", 0, "  "},
	} {
		if got := cell(row(c.day), c.hour); got != c.want {
			t.Errorf("%s %02d = %q, want %q", c.day, c.hour, got, c.want)
		}
	}
	if tue := row("Tue"); tue[len("Mon")+6*3] != '│' || tue[len("Mon")+5*3] != ' ' {
		t.Errorf("guide before 06 is missing: %q", string(tue))
	}
	if !strings.HasSuffix(string(row("Tue")), "  2") {
		t.Errorf("Tue row lacks total 2: %q", string(row("Tue")))
	}
	wantLegend := "sqrt scale, max 2 at Tue 09 (blank = 0):  ░░ 1+  ██ 2+"
	if !strings.Contains(out, wantLegend) {
		t.Errorf("legend lacks %q:\n%s", wantLegend, out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("colors without Color:\n%q", out)
	}
}

func TestHeatmapColor(t *testing.T) {
	bg := func(code int) string { return fmt.Sprintf("\x1b[48;5;%dm  %s", code, ansiReset) }
	g := axis.Fold(sample(t), axis.Spec{Y: axis.Weekday, X: axis.Hour}, 0, nil)

	for _, theme := range []Theme{Dark, Light} {
		t.Run(theme.String(), func(t *testing.T) {
			out := renderHeatmap(t, g, Options{Scale: Sqrt, Color: true, Theme: theme})
			cells := grid(out)
			colors := ramps[theme]

			for _, c := range []struct {
				what string
				code int
			}{
				{"Tue 09 (max)", colors[Levels-1]},
				{"Mon 14 (1 commit)", colors[0]},
			} {
				if !strings.Contains(cells, bg(c.code)) {
					t.Errorf("%s: cell with background %d is missing:\n%q", c.what, c.code, cells)
				}
			}
			// Night hours use the same ramp as the rest of the day.
			sun := strings.Split(cells, "\n")[7]
			if !strings.HasPrefix(sun, "Sun") || !strings.Contains(sun, bg(colors[0])) {
				t.Errorf("Sun 23 (night hour, 1 commit) is not drawn with the same ramp: %q", sun)
			}
			if strings.ContainsAny(cells, string(shades[1:])) {
				t.Errorf("shade glyphs are drawn together with colors:\n%q", cells)
			}
			if strings.Contains(out, "night") {
				t.Errorf("output mentions night hours:\n%s", out)
			}
		})
	}

	// Without an hour axis, there are no hour guides.
	g = axis.Fold(sample(t), axis.Spec{Y: axis.Project, X: axis.Weekday}, 0, nil)
	out := renderHeatmap(t, g, Options{Scale: Sqrt, Color: true})
	if strings.Contains(grid(out), "│") {
		t.Errorf("hour guides without an hour axis:\n%s", out)
	}
}

// TestHeatmapAlignsWideLabels checks that columns stay aligned with wide row labels.
func TestHeatmapAlignsWideLabels(t *testing.T) {
	g := axis.Fold(sample(t), axis.Spec{Y: axis.Project, X: axis.Hour}, 0, nil)
	lines := strings.Split(renderHeatmap(t, g, Options{}), "\n")
	jp, ascii := lines[1], lines[2]

	// The expected text does not use Width.
	// A broken Width would otherwise shift the code and the test together.
	// The Japanese label has 9 wide characters, so it is 18 wide. "abc" gets 15 spaces to match.
	const (
		jpLabel    = "日本語プロジェクト"
		asciiLabel = "abc               "
	)
	for _, c := range []struct {
		row, label, cell string
		hour             int
	}{
		{jp, jpLabel, "██", 9},        // Two commits on Tue 09.
		{ascii, asciiLabel, "░░", 14}, // One commit on Mon 14.
	} {
		rest, ok := strings.CutPrefix(c.row, c.label+" ")
		if !ok {
			t.Errorf("row = %q, want label %q followed by the cells", c.row, c.label)
			continue
		}
		// After the label, each hour is two cell characters and one separator character.
		cells := []rune(rest)
		start := c.hour * 3
		if got := string(cells[start : start+2]); got != c.cell {
			t.Errorf("%q: %02d cell = %q, want %q", c.label, c.hour, got, c.cell)
		}
	}
}

func TestHeatmapNoYAxis(t *testing.T) {
	g := axis.Fold(sample(t), axis.Spec{Y: axis.None, X: axis.Hour}, 0, nil)
	lines := strings.Split(renderHeatmap(t, g, Options{}), "\n")
	if !strings.HasPrefix(lines[1], " ") || !strings.HasSuffix(lines[1], "  4") {
		t.Errorf("single row = %q", lines[1])
	}
}

func TestHeatmapEmpty(t *testing.T) {
	g := axis.Fold(model.New(), axis.Spec{Y: axis.Weekday, X: axis.Hour}, 0, nil)
	if out := renderHeatmap(t, g, Options{}); out != "no commits\n" {
		t.Errorf("out = %q", out)
	}
}

func TestHeatmapMinRowTotal(t *testing.T) {
	g := minTotalSample(t)

	out := renderHeatmap(t, g, Options{Scale: Sqrt, MinRowTotal: 4})
	lines := strings.Split(out, "\n")
	if strings.Contains(out, "Carol") {
		t.Errorf("Carol (3 commits) is shown with MinRowTotal 4:\n%s", out)
	}
	dave := lines[1]
	if !strings.HasPrefix(dave, "Dave ") || !strings.HasSuffix(dave, "  4") {
		t.Fatalf("row = %q, want Dave with total 4", dave)
	}
	// The peak comes from the shown rows. Using Carol's 3 would make every Dave cell pale.
	if got := string([]rune(dave)[len("Dave")+1+1*3 : len("Dave")+1+1*3+2]); got != "██" {
		t.Errorf("Dave 01 = %q, want the darkest shade:\n%s", got, out)
	}
	for _, want := range []string{
		"sqrt scale, max 1 at Dave 01",
		"1 of 2 author rows hidden: fewer than 4 commits",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}

	// Zero shows every row and adds no note.
	out = renderHeatmap(t, g, Options{Scale: Sqrt})
	if !strings.Contains(out, "Carol ") || strings.Contains(out, "hidden") {
		t.Errorf("MinRowTotal 0 should show every row without a note:\n%s", out)
	}

	// When no row qualifies, only a message is written.
	out = renderHeatmap(t, g, Options{Scale: Sqrt, MinRowTotal: 100})
	if want := "no author rows with at least 100 commits (2 hidden)\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// TestScaleUsesEveryLevel checks that one commit is the lightest level and the peak is the darkest.
// It also checks that a small peak still uses every level in between.
func TestScaleUsesEveryLevel(t *testing.T) {
	for _, s := range []Scale{Sqrt, Linear, Log} {
		for _, peak := range []int{12, 100, 5000} {
			if got := s.Level(1, peak); got != 1 {
				t.Errorf("%s peak %d: Level(1) = %d, want 1", s, peak, got)
			}
			seen := map[int]bool{}
			for v := 1; v <= peak; v++ {
				seen[s.Level(v, peak)] = true
			}
			if len(seen) != Levels {
				t.Errorf("%s peak %d: uses %d of %d levels", s, peak, len(seen), Levels)
			}
		}
	}
}

func TestParseTheme(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Theme
	}{{"dark", Dark}, {" Light ", Light}} {
		if got, err := ParseTheme(c.in); err != nil || got != c.want {
			t.Errorf("ParseTheme(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	if _, err := ParseTheme("sepia"); err == nil {
		t.Error("ParseTheme(sepia) succeeded, want error")
	}
}

func renderHeatmap(t *testing.T, g axis.Grid, o Options) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Heatmap(&buf, g, o); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// grid returns the header and cells of Heatmap output, without the legend.
func grid(out string) string {
	body, _, _ := strings.Cut(out, "\n\n")
	return body
}

// minTotalSample has Carol with 3 commits at 10 and Dave with 1 commit at each of 1-4.
// Carol's cell (3) is larger than any Dave cell (1), but Dave has the larger row total.
func minTotalSample(t *testing.T) axis.Grid {
	t.Helper()
	jst := time.FixedZone("JST", 9*3600)
	a := model.New()
	a.Authors = []model.Author{{Name: "Carol"}, {Name: "Dave"}}
	a.Projects = []model.Project{{Name: "p"}}
	add := func(author uint16, hour int) {
		t.Helper()
		if err := a.Add(
			author,
			0,
			time.Date(2024, time.March, 5, hour, 0, 0, 0, jst),
			9*3600,
		); err != nil {
			t.Fatal(err)
		}
	}
	for range 3 {
		add(0, 10)
	}
	for h := 1; h <= 4; h++ {
		add(1, h)
	}
	return axis.Fold(a, axis.Spec{Y: axis.Author, X: axis.Hour}, 0, nil)
}
