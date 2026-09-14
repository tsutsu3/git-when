package render

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Levels is the number of shading levels, not counting zero.
// It comes from the shade characters, so the two never disagree.
const Levels = len(shades) - 1

// Scale maps commit counts to shading levels.
type Scale int

// Scales. The default is Sqrt.
//
// With a linear scale, the weekday daytime peak is so strong that weekends and late nights
// look the same as no commits. A square root scale keeps small counts visible.
const (
	// Sqrt uses the square root of counts.
	Sqrt Scale = iota
	// Linear uses counts as they are.
	Linear
	// Log uses log(1+count).
	Log
)

// ParseScale reads "sqrt", "linear", or "log".
func ParseScale(s string) (Scale, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sqrt":
		return Sqrt, nil
	case "linear":
		return Linear, nil
	case "log":
		return Log, nil
	}
	return Sqrt, fmt.Errorf("unknown scale %q (want sqrt, linear, log)", s)
}

// String returns the scale name used by --scale.
func (s Scale) String() string {
	switch s {
	case Linear:
		return "linear"
	case Log:
		return "log"
	default:
		return "sqrt"
	}
}

// Level maps count v to a shading level from 0 to Levels, relative to peak.
// Only zero commits give level 0. One commit gives at least level 1,
// so a few commits never look like none.
func (s Scale) Level(v, peak int) int {
	if v <= 0 || peak <= 0 {
		return 0
	}
	if v >= peak {
		return Levels
	}
	// Map 1..peak through the scale and split it evenly, so 1 is level 1 and peak is Levels.
	// Splitting 0..peak instead would put one commit at level 2 or higher when peak is small.
	lo, hi := s.apply(1), s.apply(float64(peak))
	r := (s.apply(float64(v)) - lo) / (hi - lo)
	return 1 + min(int(r*float64(Levels)), Levels-1)
}

// Steps returns the smallest count for each level, for use in a legend.
// Levels that cannot appear with a small peak are left out.
func (s Scale) Steps(peak int) []Step {
	var out []Step
	for l := 1; l <= Levels; l++ {
		lo := 1 + sort.Search(peak, func(i int) bool { return s.Level(i+1, peak) >= l })
		if lo <= peak && s.Level(lo, peak) == l {
			out = append(out, Step{Level: l, Min: lo})
		}
	}
	return out
}

func (s Scale) apply(x float64) float64 {
	switch s {
	case Linear:
		return x
	case Log:
		return math.Log1p(x)
	default:
		return math.Sqrt(x)
	}
}

// Step is one legend entry.
type Step struct {
	// Level is the shading level.
	Level int
	// Min is the smallest count shown with this level.
	Min int
}
