// Package stats computes values that do not depend on a view.
//
// Examples are the weekday and weekend split, the weekend index, and the night share.
package stats

import (
	"errors"
	"slices"
	"strings"

	"github.com/tsutsu3/git-when/internal/axis"
	"github.com/tsutsu3/git-when/internal/model"
)

// Weekend is the set of weekdays treated as the weekend. Index 0 is Monday and 6 is Sunday.
type Weekend [7]bool

// DefaultWeekend returns Saturday and Sunday.
func DefaultWeekend() Weekend {
	var w Weekend
	w[5], w[6] = true, true
	return w
}

// ParseWeekend reads a list of weekdays such as "sat,sun" or "fri, sat".
// No days, or all seven days, is an error because the weekend index would be undefined.
func ParseWeekend(s string) (Weekend, error) {
	var w Weekend
	for part := range strings.SplitSeq(s, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		d, err := axis.ParseWeekday(part)
		if err != nil {
			return Weekend{}, err
		}
		w[d] = true
	}
	switch w.Days() {
	case 0:
		return Weekend{}, errors.New("no weekend days given (e.g. sat,sun)")
	case len(w):
		return Weekend{}, errors.New("every day is a weekend day; at least one weekday must remain")
	}
	return w, nil
}

// Days returns the number of weekend days.
func (w Weekend) Days() int {
	n := 0
	for _, on := range w {
		if on {
			n++
		}
	}
	return n
}

// Names returns the weekend day names in Monday-first order.
func (w Weekend) Names() []string {
	var out []string
	for d, on := range w {
		if on {
			out = append(out, axis.WeekdayName(d))
		}
	}
	return out
}

// Week holds hourly commit counts split into weekdays and weekends.
type Week struct {
	// Weekend is the set of weekend days used for the split.
	Weekend Weekend
	// OnWeekdays holds commits per hour on weekdays.
	OnWeekdays [24]int
	// OnWeekend holds commits per hour on weekend days.
	OnWeekend [24]int
}

// SplitWeek splits the aggregate into weekdays and weekends.
// If keep is not nil, only buckets for which keep returns true are counted.
func SplitWeek(a *model.Aggregate, weekend Weekend, keep func(model.Key) bool) Week {
	w := Week{Weekend: weekend}
	for u, c := range a.Buckets {
		k := model.Unpack(u)
		if keep != nil && !keep(k) {
			continue
		}
		w.add(k, int(c))
	}
	return w
}

// WeekdayTotal returns the number of commits on weekdays.
func (w Week) WeekdayTotal() int { return sum(w.OnWeekdays[:]) }

// WeekendTotal returns the number of commits on weekend days.
func (w Week) WeekendTotal() int { return sum(w.OnWeekend[:]) }

// Total returns the number of all commits.
func (w Week) Total() int { return w.WeekdayTotal() + w.WeekendTotal() }

// EvenShare returns the weekend share if every day had the same number of commits.
// It is the number of weekend days divided by 7.
func (w Week) EvenShare() float64 {
	return float64(w.Weekend.Days()) / float64(len(w.Weekend))
}

// WeekendIndex returns the weekend share of commits divided by EvenShare.
// 1.00 means weekend days are as busy per day as weekdays. 0 means no weekend work.
// It follows the weekend definition, so --weekend fri,sat moves the baseline too.
// ok is false when there are no commits.
func (w Week) WeekendIndex() (index float64, ok bool) {
	total, days := w.Total(), w.Weekend.Days()
	if total == 0 || days == 0 || days == len(w.Weekend) {
		return 0, false
	}
	// Compute (weekend/total) / (days/7) with integers first to avoid rounding errors.
	return float64(w.WeekendTotal()*len(w.Weekend)) / float64(total*days), true
}

func (w *Week) add(k model.Key, n int) {
	if int(k.Hour) >= len(w.OnWeekdays) || int(k.Weekday) >= len(w.Weekend) {
		return
	}
	if w.Weekend[k.Weekday] {
		w.OnWeekend[k.Hour] += n
	} else {
		w.OnWeekdays[k.Hour] += n
	}
}

// ProjectSummary summarizes one repository or all repositories.
type ProjectSummary struct {
	// Name is the repository name, or "total" for all repositories.
	Name string
	// Commits is the number of commits.
	Commits int
	// Authors is the number of authors with commits.
	Authors int
	// PeakHour is the busiest hour. Ties go to the earlier hour. It is -1 without commits.
	PeakHour int
	// PeakWeekday is the busiest weekday (0=Mon). Ties go to the earlier day. It is -1 without commits.
	PeakWeekday int
	// NightShare is the share of commits in the night hours 22:00-05:59.
	NightShare float64
	// Week is the weekday and weekend split.
	Week Week
}

type tally struct {
	hours   [24]int
	days    [7]int
	authors map[uint16]struct{}
	week    Week
}

func newTally(weekend Weekend) *tally {
	return &tally{authors: map[uint16]struct{}{}, week: Week{Weekend: weekend}}
}

func (t *tally) add(k model.Key, n int) {
	if int(k.Hour) >= len(t.hours) || int(k.Weekday) >= len(t.days) {
		return
	}
	t.hours[k.Hour] += n
	t.days[k.Weekday] += n
	t.authors[k.Author] = struct{}{}
	t.week.add(k, n)
}

func (t *tally) summary(name string) ProjectSummary {
	s := ProjectSummary{
		Name: name, Authors: len(t.authors), PeakHour: -1, PeakWeekday: -1, Week: t.week,
	}
	night := 0
	for h, n := range t.hours {
		s.Commits += n
		if IsNight(h) {
			night += n
		}
	}
	if s.Commits == 0 {
		return s
	}
	s.PeakHour = argmax(t.hours[:])
	s.PeakWeekday = argmax(t.days[:])
	s.NightShare = float64(night) / float64(s.Commits)
	return s
}

// IsNight reports whether hour (0-23) is in the night hours 22:00-05:59.
func IsNight(hour int) bool {
	return hour >= 22 || hour < 6
}

// Summarize returns one summary per repository, sorted by commits in descending order.
// It also returns the summary of all repositories.
func Summarize(a *model.Aggregate, weekend Weekend) (rows []ProjectSummary, total ProjectSummary) {
	all := newTally(weekend)
	per := make([]*tally, len(a.Projects))
	for i := range per {
		per[i] = newTally(weekend)
	}
	for u, c := range a.Buckets {
		k := model.Unpack(u)
		all.add(k, int(c))
		if int(k.Project) < len(per) {
			per[k.Project].add(k, int(c))
		}
	}

	rows = make([]ProjectSummary, len(per))
	for i, t := range per {
		rows[i] = t.summary(a.Projects[i].Name)
	}
	slices.SortStableFunc(rows, func(x, y ProjectSummary) int { return y.Commits - x.Commits })
	return rows, all.summary("total")
}

// argmax returns the index of the largest value. Ties go to the smaller index.
func argmax(xs []int) int {
	best := 0
	for i, x := range xs {
		if x > xs[best] {
			best = i
		}
	}
	return best
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}
