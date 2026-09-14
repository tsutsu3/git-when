package stats

import (
	"cmp"
	"maps"
	"slices"

	"github.com/tsutsu3/git-when/internal/model"
)

// TZShift holds the distribution of UTC offsets per year.
// It can reveal moves, business trips, or a change of workplace.
// The offsets are the ones that authors recorded in their commits.
type TZShift struct {
	// Years runs from the first to the last year, including years without commits.
	Years []int
	// Offsets lists the offset columns in seconds, in ascending order.
	Offsets []int32
	// Counts holds counts as Counts[year][offset column].
	Counts [][]int
	// Other holds, per year, commits with offsets that have no column. It is nil when every offset has a column.
	Other []int
	// Total holds the number of commits per year.
	Total []int
	// Main holds the most common offset per year, counting Other too. Ties go to the smaller offset.
	Main []int32
	// MainCount holds the commits with the Main offset. Zero means the year has no commits.
	MainCount []int
}

// OffsetsByYear turns Aggregate.Offsets into a year by offset table.
// The columns are the maxColumns offsets with the most commits over all years.
// The rest go to Other. When maxColumns is zero or less, every offset gets a column.
func OffsetsByYear(a *model.Aggregate, maxColumns int) TZShift {
	if len(a.Offsets) == 0 {
		return TZShift{}
	}

	perYear := map[int]map[int32]int{}
	totals := map[int32]int{}
	minYear, maxYear := int(^uint(0)>>1), 0
	for k, c := range a.Offsets {
		y, n := int(k.Year), int(c)
		if perYear[y] == nil {
			perYear[y] = map[int32]int{}
		}
		perYear[y][k.Offset] += n
		totals[k.Offset] += n
		minYear, maxYear = min(minYear, y), max(maxYear, y)
	}

	// Pick columns by count, with ties going to the smaller offset. Show them in offset order.
	ranked := slices.SortedFunc(maps.Keys(totals), func(x, y int32) int {
		if totals[x] != totals[y] {
			return totals[y] - totals[x]
		}
		return cmp.Compare(x, y)
	})
	columns := ranked
	if maxColumns > 0 && len(columns) > maxColumns {
		columns = columns[:maxColumns]
	}
	columns = slices.Sorted(slices.Values(columns))
	index := make(map[int32]int, len(columns))
	for i, off := range columns {
		index[off] = i
	}
	hasOther := len(ranked) > len(columns)

	tz := TZShift{Offsets: columns}
	for y := minYear; y <= maxYear; y++ {
		counts := make([]int, len(columns))
		other, total, mainCount := 0, 0, 0
		var main int32
		// Offsets are visited in ascending order, so ties give the smaller offset as Main.
		for _, off := range slices.Sorted(maps.Keys(perYear[y])) {
			n := perYear[y][off]
			total += n
			if i, ok := index[off]; ok {
				counts[i] += n
			} else {
				other += n
			}
			if n > mainCount {
				main, mainCount = off, n
			}
		}
		tz.Years = append(tz.Years, y)
		tz.Counts = append(tz.Counts, counts)
		tz.Total = append(tz.Total, total)
		tz.Main = append(tz.Main, main)
		tz.MainCount = append(tz.MainCount, mainCount)
		if hasOther {
			tz.Other = append(tz.Other, other)
		}
	}
	return tz
}
