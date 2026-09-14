package axis

import (
	"slices"
	"testing"
	"time"

	"github.com/tsutsu3/git-when/internal/model"
)

var (
	jst = time.FixedZone("JST", 9*3600)
	pst = time.FixedZone("PST", -8*3600)
)

// TestIndexOutOfRange checks that indexes outside Projects and Authors are dropped.
func TestIndexOutOfRange(t *testing.T) {
	a := model.New()
	a.Projects = []model.Project{{Name: "only"}}
	k := model.Key{Project: 1, Author: 3}
	for _, kind := range []Kind{Project, Author} {
		if i := Build(kind, a, 0).Index(k); i != -1 {
			t.Errorf("Build(%d).Index(%+v) = %d, want -1", kind, k, i)
		}
	}
}

func TestParseSpec(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Spec
	}{
		{"wday/hour", Spec{Weekday, Hour}},
		{"heatmap", Spec{Weekday, Hour}},
		{" Heatmap ", Spec{Weekday, Hour}},
		{"year/month", Spec{Year, Month}},
		{"/hour", Spec{None, Hour}},
		{"none/hour", Spec{None, Hour}},
		{"Weekday / Hour", Spec{Weekday, Hour}},
		{"proj/author", Spec{Project, Author}},
	} {
		got, err := ParseSpec(c.in)
		if err != nil || got != c.want {
			t.Errorf("ParseSpec(%q) = %+v, %v; want %+v", c.in, got, err, c.want)
		}
	}
	for _, in := range []string{
		"",          // Wrong form.
		"hour",      // No slash.
		"wday/",     // No x axis.
		"/",         // No x axis.
		"hour/hour", // Same axis twice.
		"wday/day",  // Unknown axis.
		"tz/hour",   // Only for tzshift. Not a grid axis.
		"a/b/c",
	} {
		if sp, err := ParseSpec(in); err == nil {
			t.Errorf("ParseSpec(%q) = %+v, want error", in, sp)
		}
	}
}

func TestFoldWeekdayHour(t *testing.T) {
	g := Fold(sample(t), Spec{Y: Weekday, X: Hour}, 0, nil)

	if len(g.Cells) != 7 {
		t.Fatalf("rows = %d, want 7", len(g.Cells))
	}
	for i, row := range g.Cells {
		if len(row) != 24 {
			t.Fatalf("row %d has %d columns, want 24", i, len(row))
		}
	}
	if want := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}; !slices.Equal(
		labels(g.Y), want) {
		t.Errorf("y labels = %v, want %v", labels(g.Y), want)
	}
	assertCells(t, g, map[string]int{"Tue/09": 2, "Sun/23": 1, "Mon/21": 1, "Wed/00": 1})
	if g.Total != 5 || g.Max != 2 || g.PeakY != 1 || g.PeakX != 9 {
		t.Errorf("Total=%d Max=%d Peak=(%d,%d), want 5, 2, (1,9)",
			g.Total, g.Max, g.PeakY, g.PeakX)
	}
	if got, want := g.RowTotals(), []int{1, 2, 1, 0, 0, 0, 1}; !slices.Equal(got, want) {
		t.Errorf("RowTotals = %v, want %v", got, want)
	}
	cols := g.ColTotals()
	if cols[0] != 1 || cols[9] != 2 || cols[21] != 1 || cols[23] != 1 {
		t.Errorf("ColTotals = %v", cols)
	}
}

// TestFoldYearHour checks that another axis pair works without heatmap-specific code.
func TestFoldYearHour(t *testing.T) {
	g := Fold(sample(t), Spec{Y: Year, X: Hour}, 0, nil)

	// 2023 has no commits but still has a row.
	if want := []string{"2022", "2023", "2024", "2025"}; !slices.Equal(labels(g.Y), want) {
		t.Errorf("y labels = %v, want %v", labels(g.Y), want)
	}
	assertCells(t, g, map[string]int{
		"2022/00": 1, "2024/09": 2, "2024/23": 1, "2025/21": 1,
	})
}

func TestFoldWeekStartSunday(t *testing.T) {
	a := sample(t)
	mon := Fold(a, Spec{Y: Weekday, X: Hour}, 0, nil)
	sun := Fold(a, Spec{Y: Weekday, X: Hour}, 6, nil)

	if want := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}; !slices.Equal(
		labels(sun.Y), want) {
		t.Errorf("y labels = %v, want %v", labels(sun.Y), want)
	}
	// Labels and cells move together, so values looked up by label do not change.
	assertCells(t, sun, nonZero(mon))
	// The positions do move. Sunday is the first row and Tuesday is the third.
	if sun.Cells[0][23] != 1 || sun.Cells[2][9] != 2 {
		t.Errorf("Sun/23 at row 0 = %d, Tue/09 at row 2 = %d; want 1, 2",
			sun.Cells[0][23], sun.Cells[2][9])
	}
	if sun.Total != mon.Total || sun.Max != mon.Max {
		t.Errorf("Total/Max changed: %d/%d vs %d/%d", sun.Total, sun.Max, mon.Total, mon.Max)
	}
	// Axes without weekdays are not affected.
	if !slices.Equal(labels(Build(Hour, a, 6)), labels(Build(Hour, a, 0))) {
		t.Error("week start changed the hour axis")
	}
}

func TestFoldNoYAxis(t *testing.T) {
	g := Fold(sample(t), Spec{Y: None, X: Hour}, 0, nil)
	if len(g.Cells) != 1 || g.Y.Name() != "" {
		t.Fatalf("rows = %d, name = %q; want 1 unnamed row", len(g.Cells), g.Y.Name())
	}
	assertCells(t, g, map[string]int{"/00": 1, "/09": 2, "/21": 1, "/23": 1})
}

func TestFoldKeep(t *testing.T) {
	bob := func(k model.Key) bool { return k.Author == 1 }
	g := Fold(sample(t), Spec{Y: Project, X: Author}, 0, bob)
	if want := []string{"p0", "p1"}; !slices.Equal(labels(g.Y), want) {
		t.Errorf("y labels = %v, want %v", labels(g.Y), want)
	}
	// Axes are built before filtering, so the Alice column stays.
	assertCells(t, g, map[string]int{"p0/Bob": 1, "p1/Bob": 1})
}

func TestFoldEmpty(t *testing.T) {
	g := Fold(model.New(), Spec{Y: Year, X: Hour}, 0, nil)
	if g.Y.Len() != 0 || g.Total != 0 || g.PeakY != -1 || g.PeakX != -1 {
		t.Errorf("empty fold: years=%d Total=%d Peak=(%d,%d)",
			g.Y.Len(), g.Total, g.PeakY, g.PeakX)
	}
	if cols := g.ColTotals(); len(cols) != 24 {
		t.Errorf("ColTotals len = %d, want 24", len(cols))
	}
}

func TestWeekdayName(t *testing.T) {
	for d, want := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		if got := WeekdayName(d); got != want {
			t.Errorf("WeekdayName(%d) = %q, want %q", d, got, want)
		}
	}
	for _, d := range []int{-1, 7} {
		if got := WeekdayName(d); got != "" {
			t.Errorf("WeekdayName(%d) = %q, want empty", d, got)
		}
	}
}

func TestParseWeekday(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int
	}{{"mon", 0}, {"Sun", 6}, {"sunday", 6}, {" FRI ", 4}} {
		if got, err := ParseWeekday(c.in); err != nil || got != c.want {
			t.Errorf("ParseWeekday(%q) = %d, %v; want %d", c.in, got, err, c.want)
		}
	}
	for _, in := range []string{"", "su", "sundays", "7"} {
		if _, err := ParseWeekday(in); err == nil {
			t.Errorf("ParseWeekday(%q) succeeded, want error", in)
		}
	}
}

// sample returns an aggregate with five commits of known weekday, hour, year, author, and project.
// 2023 has no commits. This tests empty years on the year axis.
func sample(t *testing.T) *model.Aggregate {
	t.Helper()
	a := model.New()
	a.Authors = []model.Author{{Name: "Alice"}, {Name: "Bob"}}
	a.Projects = []model.Project{{Name: "p0"}, {Name: "p1"}}
	for _, c := range []struct {
		author, project uint16
		ts              time.Time
	}{
		{0, 0, time.Date(2024, time.March, 5, 9, 0, 0, 0, jst)},    // Tue 09
		{0, 0, time.Date(2024, time.March, 5, 9, 59, 0, 0, jst)},   // Tue 09
		{0, 1, time.Date(2024, time.March, 10, 23, 0, 0, 0, jst)},  // Sun 23
		{1, 1, time.Date(2025, time.January, 6, 21, 0, 0, 0, pst)}, // Mon 21
		{1, 0, time.Date(2022, time.June, 1, 0, 0, 0, 0, jst)},     // Wed 00
	} {
		_, off := c.ts.Zone()
		//nolint:gosec // Fixed test offsets fit in int32.
		if err := a.Add(c.author, c.project, c.ts, int32(off)); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

// nonZero returns the non-zero cells of a Grid, keyed by "row/column" labels.
func nonZero(g Grid) map[string]int {
	out := map[string]int{}
	for iy, row := range g.Cells {
		for ix, v := range row {
			if v != 0 {
				out[g.Y.Label(iy)+"/"+g.X.Label(ix)] = v
			}
		}
	}
	return out
}

func labels(a Axis) []string {
	out := make([]string, a.Len())
	for i := range out {
		out[i] = a.Label(i)
	}
	return out
}

func assertCells(t *testing.T, g Grid, want map[string]int) {
	t.Helper()
	got := nonZero(g)
	if len(got) != len(want) {
		t.Errorf("non-zero cells = %v, want %v", got, want)
		return
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("cell %s = %d, want %d (all: %v)", k, got[k], v, got)
		}
	}
}
