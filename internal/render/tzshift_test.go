package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tsutsu3/git-when/internal/stats"
)

func TestFormatOffset(t *testing.T) {
	for sec, want := range map[int32]string{
		9 * 3600:          "+09:00",
		-8 * 3600:         "-08:00",
		5*3600 + 45*60:    "+05:45",
		0:                 "+00:00",
		-(3*3600 + 30*60): "-03:30",
		14 * 3600:         "+14:00",
	} {
		if got := FormatOffset(sec); got != want {
			t.Errorf("FormatOffset(%d) = %q, want %q", sec, got, want)
		}
	}
}

func TestTZShift(t *testing.T) {
	tz := stats.TZShift{
		Years:     []int{2020, 2021, 2022, 2023},
		Offsets:   []int32{-8 * 3600, 9 * 3600},
		Counts:    [][]int{{0, 3}, {2, 1}, {0, 0}, {0, 0}},
		Other:     []int{0, 0, 0, 1},
		Total:     []int{3, 3, 0, 1},
		Main:      []int32{9 * 3600, -8 * 3600, 0, 5*3600 + 45*60},
		MainCount: []int{3, 2, 0, 1},
	}
	var buf bytes.Buffer
	if err := TZShift(&buf, tz, Options{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	lines := strings.Split(out, "\n")

	header := lines[0]
	if want := "year  -08:00  +09:00  other  total  most common"; header != want {
		t.Fatalf("header = %q, want %q", header, want)
	}
	row := func(year string) string {
		t.Helper()
		for _, l := range lines[1:] {
			if strings.HasPrefix(l, year+" ") || l == year {
				return l
			}
		}
		t.Fatalf("no row %s:\n%s", year, out)
		return ""
	}
	// Number columns are right-aligned with their headings. The output is ASCII only.
	at := func(l, column string) byte {
		t.Helper()
		end := strings.Index(header, column) + len(column)
		if end > len(l) {
			return ' '
		}
		return l[end-1]
	}
	for _, c := range []struct {
		year, column string
		want         byte
	}{
		{"2020", "+09:00", '3'},
		{"2020", "-08:00", ' '}, // Zero is blank.
		{"2021", "-08:00", '2'},
		{"2021", "+09:00", '1'},
		{"2021", "total", '3'},
		{"2022", "total", '0'},
		{"2023", "other", '1'},
	} {
		if got := at(row(c.year), c.column); got != c.want {
			t.Errorf("%s %s = %q, want %q\n%s", c.year, c.column, got, c.want, out)
		}
	}

	for _, c := range []struct{ year, suffix string }{
		{"2020", "+09:00 100%"},
		{"2021", "-08:00  67%  changed"},
		{"2022", "0"}, // A year without commits has an empty most common column.
		// The comparison skips empty years and uses the last year with commits.
		{"2023", "+05:45 100%  changed"},
	} {
		if got := row(c.year); !strings.HasSuffix(got, c.suffix) {
			t.Errorf("%s row = %q, want suffix %q", c.year, got, c.suffix)
		}
	}
	wantFooter := "UTC offsets as recorded by the authors; offsets beyond the 2 most used are in other"
	if !strings.Contains(out, wantFooter) {
		t.Errorf("footer lacks %q:\n%s", wantFooter, out)
	}

	// Without offsets outside the columns, there is no other column and no note.
	tz.Offsets, tz.Other = []int32{-8 * 3600, 5*3600 + 45*60, 9 * 3600}, nil
	tz.Counts = [][]int{{0, 0, 3}, {2, 0, 1}, {0, 0, 0}, {0, 1, 0}}
	buf.Reset()
	if err := TZShift(&buf, tz, Options{}); err != nil {
		t.Fatal(err)
	}
	if out := buf.String(); strings.Contains(out, "other") {
		t.Errorf("other appears without hidden offsets:\n%s", out)
	}
}

func TestTZShiftEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := TZShift(&buf, stats.TZShift{}, Options{}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "no commits\n" {
		t.Errorf("out = %q", buf.String())
	}
}
