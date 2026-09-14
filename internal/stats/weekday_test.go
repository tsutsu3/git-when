package stats

import (
	"math"
	"testing"
	"time"

	"github.com/tsutsu3/git-when/internal/model"
)

func TestByWeekday(t *testing.T) {
	a := aggregate(t, []string{"p0", "p1"}, []commit{
		{project: 0, at: time.Date(2024, time.March, 5, 9, 0, 0, 0, jst)},   // Tue 09
		{project: 0, at: time.Date(2024, time.March, 5, 9, 30, 0, 0, jst)},  // Tue 09
		{project: 1, at: time.Date(2024, time.March, 5, 23, 0, 0, 0, jst)},  // Tue 23
		{project: 1, at: time.Date(2024, time.March, 10, 14, 0, 0, 0, jst)}, // Sun 14
	})

	days := ByWeekday(a, nil)
	for d, day := range days {
		if day.Weekday != d {
			t.Errorf("days[%d].Weekday = %d", d, day.Weekday)
		}
	}
	check := func(d, commits, peak int, night float64) {
		t.Helper()
		got := days[d]
		if got.Commits != commits || got.PeakHour != peak ||
			math.Abs(got.NightShare-night) > 1e-12 {
			t.Errorf(
				"days[%d] = %+v; want commits=%d peak=%d night=%v",
				d,
				got,
				commits,
				peak,
				night,
			)
		}
	}
	check(0, 0, -1, 0)    // Mon has no commits.
	check(1, 3, 9, 1.0/3) // Tue has a night commit at 23.
	check(6, 1, 14, 0)    // Sun
	check(5, 0, -1, 0)    // Sat

	// Hourly counts per weekday, used by the days view.
	if tue := days[1].Hours; tue[9] != 2 || tue[23] != 1 || tue[10] != 0 {
		t.Errorf("Tue hours: [9]=%d [23]=%d [10]=%d; want 2, 1, 0", tue[9], tue[23], tue[10])
	}
	if sun := days[6].Hours; sun[14] != 1 {
		t.Errorf("Sun hours[14] = %d, want 1", sun[14])
	}

	onlyP1 := ByWeekday(a, func(k model.Key) bool { return k.Project == 1 })
	if onlyP1[1].Commits != 1 || onlyP1[1].PeakHour != 23 {
		t.Errorf("keep project 1: Tue = %+v; want 1 commit at 23", onlyP1[1])
	}
}
