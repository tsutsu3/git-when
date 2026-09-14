package stats

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/tsutsu3/git-when/internal/model"
)

const (
	offJST = 9 * 3600
	offPST = -8 * 3600
	offNPT = 5*3600 + 45*60
)

func TestOffsetsByYear(t *testing.T) {
	a := model.New()
	add := func(year int, offset int32, n int) {
		t.Helper()
		at := time.Date(year, time.June, 1, 12, 0, 0, 0, time.FixedZone("", int(offset)))
		for range n {
			if err := a.Add(0, 0, at, offset); err != nil {
				t.Fatal(err)
			}
		}
	}
	add(2020, offJST, 3)
	add(2021, offJST, 1)
	add(2021, offPST, 2) // PST leads in 2021.
	add(2023, offNPT, 1) // No commits in 2022.
	add(2024, offJST, 1)
	add(2024, offPST, 1) // On a tie the smaller offset (PST) wins.

	// Totals are JST 5, PST 3, and NPT 1. The top two get columns and NPT goes to other.
	got := OffsetsByYear(a, 2)
	want := TZShift{
		Years:     []int{2020, 2021, 2022, 2023, 2024},
		Offsets:   []int32{offPST, offJST},
		Counts:    [][]int{{0, 3}, {2, 1}, {0, 0}, {0, 0}, {1, 1}},
		Other:     []int{0, 0, 0, 1, 0},
		Total:     []int{3, 3, 0, 1, 2},
		Main:      []int32{offJST, offPST, 0, offNPT, offPST},
		MainCount: []int{3, 2, 0, 1, 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("OffsetsByYear(a, 2) =\n%+v\nwant\n%+v", got, want)
	}

	// With enough columns, every offset gets one and there is no other.
	all := OffsetsByYear(a, 0)
	wantAll := []int32{offPST, offNPT, offJST}
	if !slices.Equal(all.Offsets, wantAll) || all.Other != nil {
		t.Errorf("OffsetsByYear(a, 0): Offsets=%v Other=%v; want %v and nil",
			all.Offsets, all.Other, wantAll)
	}

	if empty := OffsetsByYear(model.New(), 8); !reflect.DeepEqual(empty, TZShift{}) {
		t.Errorf("empty aggregate = %+v, want zero value", empty)
	}
}
