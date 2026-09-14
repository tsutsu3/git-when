package stats

import "github.com/tsutsu3/git-when/internal/model"

// Day holds the counts of one weekday.
type Day struct {
	// Weekday is the day number, 0=Mon..6=Sun.
	Weekday int
	// Hours holds commits per hour.
	Hours [24]int
	// Commits is the number of commits on this day.
	Commits int
	// PeakHour is the busiest hour. Ties go to the earlier hour. It is -1 without commits.
	PeakHour int
	// NightShare is the share of commits in the night hours 22:00-05:59.
	NightShare float64
}

// ByWeekday returns the counts of each weekday in 0=Mon..6=Sun order.
// If keep is not nil, only buckets for which keep returns true are counted.
func ByWeekday(a *model.Aggregate, keep func(model.Key) bool) [7]Day {
	var hours [7][24]int
	for u, c := range a.Buckets {
		k := model.Unpack(u)
		if keep != nil && !keep(k) {
			continue
		}
		if int(k.Weekday) >= len(hours) || int(k.Hour) >= len(hours[0]) {
			continue
		}
		hours[k.Weekday][k.Hour] += int(c)
	}

	var days [7]Day
	for d := range days {
		day := Day{Weekday: d, Hours: hours[d], PeakHour: -1}
		night := 0
		for h, n := range hours[d] {
			day.Commits += n
			if IsNight(h) {
				night += n
			}
		}
		if day.Commits > 0 {
			day.PeakHour = argmax(hours[d][:])
			day.NightShare = float64(night) / float64(day.Commits)
		}
		days[d] = day
	}
	return days
}
