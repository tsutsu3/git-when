package render

import (
	"fmt"
	"strings"
)

// Theme is the brightness of the terminal background.
// Heatmap colors move away from the background as counts grow.
type Theme int

// Terminal backgrounds. The default is Dark.
const (
	// Dark is a dark terminal background.
	Dark Theme = iota
	// Light is a light terminal background.
	Light
)

// ParseTheme reads "dark" or "light".
func ParseTheme(s string) (Theme, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "dark":
		return Dark, nil
	case "light":
		return Light, nil
	}
	return Dark, fmt.Errorf("unknown theme %q (want dark, light)", s)
}

// String returns "dark" or "light".
func (t Theme) String() string {
	if t == Light {
		return "light"
	}
	return "dark"
}

// ramp holds xterm-256 colors of one hue.
// ramp[level-1] is the background color for a shading level.
type ramp [Levels]int

// ramps holds the shading colors for each terminal background.
// The amount is shown by lightness within one orange hue. Every hour of the day uses the same ramp.
// The SVG uses the same ramps, so both outputs look alike.
//
// The ramps follow these rules.
//   - Each ramp changes OKLCH lightness steadily and keeps the hue almost constant.
//   - Larger counts are further from the background.
//     They get lighter on dark backgrounds and darker on light backgrounds.
//   - Even level 1 (one commit) has at least 2:1 contrast with the background,
//     so it differs from a blank cell.
var ramps = [...]ramp{
	Dark:  {130, 173, 215, 223}, // #af5f00 #d7875f #ffaf5f #ffd7af
	Light: {209, 166, 124, 52},  // #ff875f #d75f00 #af0000 #5f0000
}

// heatCell returns a cell for a shading level, width columns wide. Zero is blank.
// With colors it uses a background color. Without colors it uses shade characters.
func heatCell(level, width int, o Options) string {
	if level <= 0 {
		return strings.Repeat(" ", width)
	}
	level = min(level, Levels)
	if !o.Color {
		return strings.Repeat(string(shades[level]), width)
	}
	code := ramps[o.Theme][level-1] //nolint:gosec // level was clamped to 1..Levels above.
	return fmt.Sprintf("\x1b[48;5;%dm%s%s", code, strings.Repeat(" ", width), ansiReset)
}
