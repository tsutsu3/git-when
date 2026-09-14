// Package axis chooses which two of the six bucket dimensions become the rows and columns of a grid.
//
// Every grid view is the same operation. Pick two axes and sum over the rest.
//
//	-p wday/hour   is the heatmap view
//	-p year/month  is the month view
//	-p /hour       is the hour view (no y axis)
//	-p year/hour   shows how working hours change over the years (no view name)
//
// Adding an axis adds every combination with it. No new renderer is needed.
package axis

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tsutsu3/git-when/internal/model"
)

const axisList = "hour, wday, month, year, project, author"

var kindNames = map[string]Kind{
	"":        None,
	"none":    None,
	"hour":    Hour,
	"wday":    Weekday,
	"weekday": Weekday,
	"month":   Month,
	"year":    Year,
	"project": Project,
	"proj":    Project,
	"author":  Author,
}

// aliases maps named views to their axes.
var aliases = map[string]Spec{
	"heatmap": {Y: Weekday, X: Hour},
}

var weekdayNames = [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

var fullWeekdayNames = [7]string{
	"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday",
}

// Kind is the type of an axis.
type Kind int

// Axis kinds. None is used for a grid without a y axis (a single row).
const (
	None Kind = iota
	Hour
	Weekday
	Month
	Year
	Project
	Author
)

func parseKind(s string) (Kind, error) {
	k, ok := kindNames[strings.TrimSpace(s)]
	if !ok {
		return None, fmt.Errorf("unknown axis %q (want one of %s)", s, axisList)
	}
	return k, nil
}

// Axis is one axis of a grid.
// Len and Label describe its ticks. Index places a bucket on it.
type Axis struct {
	// Kind is the type of the axis.
	Kind   Kind
	name   string
	labels []string
	index  func(model.Key) int // -1 when the key is out of range.
}

// Build creates an axis of kind k.
// Year, project, and author ranges depend on the data, so Build reads them from a.
// weekStart (0=Mon..6=Sun) changes only the order of the weekday axis, not the stored data.
func Build(k Kind, a *model.Aggregate, weekStart int) Axis {
	switch k {
	case None:
		return Axis{
			Kind: None, labels: []string{""},
			index: func(model.Key) int { return 0 },
		}

	case Hour:
		lb := make([]string, 24)
		for i := range lb {
			lb[i] = fmt.Sprintf("%02d", i)
		}
		return Axis{
			Kind: Hour, name: "hour", labels: lb,
			index: func(k model.Key) int { return int(k.Hour) },
		}

	case Weekday:
		// Storage always uses Monday=0. Position i shows weekday (weekStart+i)%7.
		start := ((weekStart % 7) + 7) % 7
		var pos [7]int
		lb := make([]string, 7)
		for i := range 7 {
			d := (start + i) % 7
			pos[d] = i
			lb[i] = weekdayNames[d]
		}
		return Axis{
			Kind: Weekday, name: "weekday", labels: lb,
			index: func(k model.Key) int {
				if int(k.Weekday) >= len(pos) {
					return -1
				}
				return pos[k.Weekday]
			},
		}

	case Month:
		lb := make([]string, 12)
		for i := range lb {
			lb[i] = strconv.Itoa(i + 1)
		}
		return Axis{
			Kind: Month, name: "month", labels: lb,
			index: func(k model.Key) int {
				if int(k.Month) >= len(lb) {
					return -1
				}
				return int(k.Month)
			},
		}

	case Year:
		// Include empty years between the first and last year.
		// Skipping them would distort changes over time.
		var first, n int
		if years := a.Years(); len(years) > 0 {
			first, n = years[0], years[len(years)-1]-years[0]+1
		}
		lb := make([]string, n)
		for i := range lb {
			lb[i] = strconv.Itoa(first + i)
		}
		return Axis{
			Kind: Year, name: "year", labels: lb,
			index: func(k model.Key) int {
				return inRange(int(k.Year)-first, n)
			},
		}

	case Project:
		lb := make([]string, len(a.Projects))
		for i, p := range a.Projects {
			lb[i] = p.Name
		}
		return Axis{
			Kind: Project, name: "project", labels: lb,
			index: func(k model.Key) int { return inRange(int(k.Project), len(lb)) },
		}

	case Author:
		lb := make([]string, len(a.Authors))
		for i, au := range a.Authors {
			lb[i] = au.Name
		}
		return Axis{
			Kind: Author, name: "author", labels: lb,
			index: func(k model.Key) int { return inRange(int(k.Author), len(lb)) },
		}
	}
	panic(fmt.Sprintf("axis: unknown kind %d", k))
}

// Name returns the axis name, such as "hour". It is empty for None.
func (a Axis) Name() string { return a.name }

// Len returns the number of ticks.
func (a Axis) Len() int { return len(a.labels) }

// Label returns the label of tick i. It is empty when i is out of range.
func (a Axis) Label(i int) string {
	if i < 0 || i >= len(a.labels) {
		return ""
	}
	return a.labels[i]
}

// Index returns the tick of k on this axis. It returns -1 when k is not on the axis.
func (a Axis) Index(k model.Key) int { return a.index(k) }

// Spec is an axis spec in "y/x" form.
// Y is None for a grid without a y axis (a one-row histogram).
type Spec struct{ Y, X Kind }

// ParseSpec reads a spec such as "wday/hour" or an alias such as "heatmap".
func ParseSpec(s string) (Spec, error) {
	norm := strings.ToLower(strings.TrimSpace(s))
	if sp, ok := aliases[norm]; ok {
		return sp, nil
	}
	y, x, ok := strings.Cut(norm, "/")
	if !ok {
		return Spec{}, fmt.Errorf(
			"axis spec must be y/x (e.g. wday/hour, or /hour for no y axis): %q", s)
	}
	ky, err := parseKind(y)
	if err != nil {
		return Spec{}, err
	}
	kx, err := parseKind(x)
	if err != nil {
		return Spec{}, err
	}
	if kx == None {
		return Spec{}, fmt.Errorf("x axis is required (e.g. /hour): %q", s)
	}
	if ky == kx {
		return Spec{}, fmt.Errorf("y and x axes must differ: %q", s)
	}
	return Spec{Y: ky, X: kx}, nil
}

// Grid is an aggregate folded onto two axes.
// It is the shared intermediate form of every grid view.
type Grid struct {
	// Y and X are the row and column axes.
	Y, X Axis
	// Cells holds the counts as Cells[y][x].
	Cells [][]int
	// Max is the largest cell value.
	Max int
	// Total is the sum of all cells.
	Total int
	// PeakY and PeakX locate the Max cell. Both are -1 when every cell is zero.
	PeakY int
	PeakX int
}

// Fold folds a onto the two axes of sp.
// If keep is not nil, only buckets for which keep returns true are counted.
// Callers use keep to limit the grid to a project, author, or year.
func Fold(a *model.Aggregate, sp Spec, weekStart int, keep func(model.Key) bool) Grid {
	y := Build(sp.Y, a, weekStart)
	x := Build(sp.X, a, weekStart)

	cells := make([][]int, y.Len())
	for i := range cells {
		cells[i] = make([]int, x.Len())
	}

	g := Grid{Y: y, X: x, Cells: cells, PeakY: -1, PeakX: -1}
	for u, c := range a.Buckets {
		k := model.Unpack(u)
		if keep != nil && !keep(k) {
			continue
		}
		iy, ix := y.Index(k), x.Index(k)
		if iy < 0 || ix < 0 {
			continue
		}
		cells[iy][ix] += int(c)
		g.Total += int(c)
	}
	// Scan in a fixed order so that ties do not depend on map order. The first peak wins.
	for iy := range cells {
		for ix, v := range cells[iy] {
			if v > g.Max {
				g.Max, g.PeakY, g.PeakX = v, iy, ix
			}
		}
	}
	return g
}

// RowTotals returns the total of each row. Legends and total columns use it.
func (g Grid) RowTotals() []int {
	out := make([]int, len(g.Cells))
	for i, row := range g.Cells {
		for _, v := range row {
			out[i] += v
		}
	}
	return out
}

// ColTotals returns the total of each column.
func (g Grid) ColTotals() []int {
	out := make([]int, g.X.Len())
	for _, row := range g.Cells {
		for i, v := range row {
			out[i] += v
		}
	}
	return out
}

// WeekdayName returns the short English name of weekday d (0=Mon..6=Sun).
// It is empty when d is out of range.
func WeekdayName(d int) string {
	if d < 0 || d >= len(weekdayNames) {
		return ""
	}
	return weekdayNames[d]
}

// ParseWeekday converts a name such as "sun" or "Sunday" to 0=Mon..6=Sun.
// It is used for --week-start and --weekend.
func ParseWeekday(s string) (int, error) {
	norm := strings.ToLower(strings.TrimSpace(s))
	for i := range weekdayNames {
		if norm == strings.ToLower(weekdayNames[i]) || norm == fullWeekdayNames[i] {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unknown weekday %q (want mon..sun)", s)
}

func inRange(i, n int) int {
	if i < 0 || i >= n {
		return -1
	}
	return i
}
