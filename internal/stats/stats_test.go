package stats

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/tsutsu3/git-when/internal/model"
)

var jst = time.FixedZone("JST", 9*3600)

type commit struct {
	author, project uint16
	at              time.Time
}

func TestParseWeekend(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"sat,sun", []string{"Sat", "Sun"}},
		{"Sun, Sat", []string{"Sat", "Sun"}},
		{"fri,sat,", []string{"Fri", "Sat"}},
		{"friday,saturday,sat", []string{"Fri", "Sat"}},
	} {
		w, err := ParseWeekend(c.in)
		if err != nil {
			t.Errorf("ParseWeekend(%q): %v", c.in, err)
			continue
		}
		if got := w.Names(); !slices.Equal(got, c.want) {
			t.Errorf("ParseWeekend(%q).Names() = %q, want %q", c.in, got, c.want)
		}
	}
	for _, in := range []string{"", ",", "sat,funday", "mon,tue,wed,thu,fri,sat,sun"} {
		if w, err := ParseWeekend(in); err == nil {
			t.Errorf("ParseWeekend(%q) = %v, want error", in, w)
		}
	}
}

func TestSplitWeek(t *testing.T) {
	a := aggregate(t, []string{"p0", "p1"}, []commit{
		{project: 0, at: time.Date(2024, time.March, 5, 9, 0, 0, 0, jst)},   // Tue 09
		{project: 0, at: time.Date(2024, time.March, 9, 23, 0, 0, 0, jst)},  // Sat 23
		{project: 1, at: time.Date(2024, time.March, 10, 23, 0, 0, 0, jst)}, // Sun 23
	})

	w := SplitWeek(a, DefaultWeekend(), nil)
	if w.OnWeekdays[9] != 1 || w.OnWeekend[23] != 2 || w.Total() != 3 {
		t.Errorf("weekday[9]=%d weekend[23]=%d total=%d; want 1, 2, 3",
			w.OnWeekdays[9], w.OnWeekend[23], w.Total())
	}

	// Another weekend definition splits the same commits differently.
	friSat, err := ParseWeekend("fri,sat")
	if err != nil {
		t.Fatal(err)
	}
	w = SplitWeek(a, friSat, nil)
	if w.OnWeekdays[23] != 1 || w.OnWeekend[23] != 1 {
		t.Errorf("fri,sat: weekday[23]=%d weekend[23]=%d; want 1, 1",
			w.OnWeekdays[23], w.OnWeekend[23])
	}

	w = SplitWeek(a, DefaultWeekend(), func(k model.Key) bool { return k.Project == 1 })
	if w.Total() != 1 || w.OnWeekend[23] != 1 {
		t.Errorf("keep project 1: total=%d weekend[23]=%d; want 1, 1", w.Total(), w.OnWeekend[23])
	}
}

// TestWeekendIndexEven checks that one commit per weekday gives an index of exactly 1.00.
func TestWeekendIndexEven(t *testing.T) {
	a := aggregate(t, []string{"p"}, oneCommitPerDay(10))
	for _, spec := range []string{"sat,sun", "fri,sat", "sun", "mon,tue,wed"} {
		weekend, err := ParseWeekend(spec)
		if err != nil {
			t.Fatal(err)
		}
		idx, ok := SplitWeek(a, weekend, nil).WeekendIndex()
		if !ok || idx != 1 {
			t.Errorf("--weekend %s: WeekendIndex = %v, %v; want exactly 1, true", spec, idx, ok)
		}
	}
}

func TestWeekendIndex(t *testing.T) {
	sat := time.Date(2024, time.March, 9, 10, 0, 0, 0, jst)
	mon := time.Date(2024, time.March, 4, 10, 0, 0, 0, jst)
	cases := []struct {
		name    string
		commits []commit
		want    float64
		ok      bool
	}{
		{"no commits", nil, 0, false},
		{"weekdays only", []commit{{at: mon}, {at: mon}}, 0, true},
		// All commits on the weekend give 1 / (2/7) = 3.5.
		{"weekend only", []commit{{at: sat}}, 3.5, true},
		// A weekend share of 1/4 gives (1/4) / (2/7) = 7/8.
		{"one in four", []commit{{at: sat}, {at: mon}, {at: mon}, {at: mon}}, 0.875, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := SplitWeek(aggregate(t, []string{"p"}, c.commits), DefaultWeekend(), nil)
			got, ok := w.WeekendIndex()
			if ok != c.ok || math.Abs(got-c.want) > 1e-12 {
				t.Errorf("WeekendIndex = %v, %v; want %v, %v", got, ok, c.want, c.ok)
			}
		})
	}
}

func TestIsNight(t *testing.T) {
	var night []int
	for h := range 24 {
		if IsNight(h) {
			night = append(night, h)
		}
	}
	if want := []int{0, 1, 2, 3, 4, 5, 22, 23}; !slices.Equal(night, want) {
		t.Errorf("night hours = %v, want %v", night, want)
	}
}

func TestSummarize(t *testing.T) {
	a := aggregate(t, []string{"a", "b", "empty"}, []commit{
		{0, 0, time.Date(2024, time.March, 5, 9, 0, 0, 0, jst)},   // a, Alice, Tue 09
		{0, 0, time.Date(2024, time.March, 5, 9, 30, 0, 0, jst)},  // a, Alice, Tue 09
		{0, 0, time.Date(2024, time.March, 10, 23, 0, 0, 0, jst)}, // a, Alice, Sun 23
		{1, 1, time.Date(2024, time.March, 4, 21, 0, 0, 0, jst)},  // b, Bob, Mon 21
		{0, 1, time.Date(2024, time.March, 6, 0, 0, 0, 0, jst)},   // b, Alice, Wed 00
	})

	rows, total := Summarize(a, DefaultWeekend())

	if got := []string{rows[0].Name, rows[1].Name, rows[2].Name}; !slices.Equal(
		got, []string{"a", "b", "empty"}) {
		t.Fatalf("order = %q, want most commits first", got)
	}
	check := func(s ProjectSummary, commits, authors, peakHour, peakDay int, night float64) {
		t.Helper()
		if s.Commits != commits || s.Authors != authors || s.PeakHour != peakHour ||
			s.PeakWeekday != peakDay || math.Abs(s.NightShare-night) > 1e-12 {
			t.Errorf("%s = %+v; want commits=%d authors=%d peakHour=%d peakDay=%d night=%v",
				s.Name, s, commits, authors, peakHour, peakDay, night)
		}
	}
	check(rows[0], 3, 1, 9, 1, 1.0/3) // 23 is a night hour.
	check(rows[1], 2, 2, 0, 0, 0.5)   // Ties go to the earlier hour (0) and day (Mon).
	check(rows[2], 0, 0, -1, -1, 0)
	check(total, 5, 2, 9, 1, 2.0/5)

	// Only the Sunday commit is on the weekend. (1/3) / (2/7) = 7/6.
	if idx, ok := rows[0].Week.WeekendIndex(); !ok || math.Abs(idx-7.0/6) > 1e-12 {
		t.Errorf("a: WeekendIndex = %v, %v; want 7/6", idx, ok)
	}
	if _, ok := rows[2].Week.WeekendIndex(); ok {
		t.Error("empty: WeekendIndex ok = true, want false")
	}
}

func aggregate(t *testing.T, projects []string, commits []commit) *model.Aggregate {
	t.Helper()
	a := model.New()
	for _, name := range projects {
		a.Projects = append(a.Projects, model.Project{Name: name})
	}
	for _, c := range commits {
		if err := a.Add(c.author, c.project, c.at, 9*3600); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

// oneCommitPerDay returns one commit at hour on each of the 7 days from 2024-03-04 (Mon).
func oneCommitPerDay(hour int) []commit {
	var cs []commit
	for d := range 7 {
		cs = append(cs, commit{at: time.Date(2024, time.March, 4+d, hour, 0, 0, 0, jst)})
	}
	return cs
}
