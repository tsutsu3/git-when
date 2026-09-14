// Package progress shows the progress of slow work on one terminal line.
//
// Without feedback, a user cannot tell a frozen program from a slow one.
// The progress line keeps rewriting one stderr line and is cleared at the end.
package progress

import (
	"fmt"
	"io"
	"time"

	"github.com/tsutsu3/git-when/internal/render"
)

const (
	// interval is the shortest time between two Update redraws.
	// It keeps terminal writes from slowing the work down.
	interval = 100 * time.Millisecond
	// maxWidth limits the display width of the line.
	// A wrapped line cannot be rewritten and would add new lines.
	maxWidth = 78
	// clearLine returns to the start of the line and clears it.
	clearLine = "\r\x1b[K"
)

// Reporter rewrites one line with progress. A disabled Reporter does nothing.
type Reporter struct {
	w       io.Writer
	enabled bool
	now     func() time.Time
	last    time.Time
	shown   bool // Whether a progress line is on the screen.
}

// New returns a Reporter that writes to w. It writes nothing when enabled is false.
func New(w io.Writer, enabled bool) *Reporter {
	return &Reporter{w: w, enabled: enabled, now: time.Now}
}

// Update redraws the line if at least interval has passed since the last redraw.
// Use it where calls come quickly, such as once per directory.
func (r *Reporter) Update(format string, args ...any) {
	if !r.enabled {
		return
	}
	now := r.now()
	if r.shown && now.Sub(r.last) < interval {
		return
	}
	r.write(now, fmt.Sprintf(format, args...))
}

// Set redraws the line every time.
// Use it before each slow step, such as once per repository.
func (r *Reporter) Set(format string, args ...any) {
	if !r.enabled {
		return
	}
	r.write(r.now(), fmt.Sprintf(format, args...))
}

// Clear removes the progress line.
// Call it before other output such as warnings, and at the end of the work.
func (r *Reporter) Clear() {
	if !r.enabled || !r.shown {
		return
	}
	_, _ = io.WriteString(r.w, clearLine)
	r.shown = false
}

func (r *Reporter) write(now time.Time, text string) {
	_, _ = io.WriteString(r.w, clearLine+render.Truncate(text, maxWidth))
	r.last, r.shown = now, true
}

// TruncateLeft keeps the end of s and replaces the start with "…" when s is wider than w.
// For paths, the end shows where the work is, so the start is cut instead.
func TruncateLeft(s string, w int) string {
	if render.Width(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	runes := []rune(s)
	used, start := 0, len(runes)
	for start > 0 {
		rw := render.Width(string(runes[start-1]))
		if used+rw > w-1 {
			break
		}
		used += rw
		start--
	}
	return "…" + string(runes[start:])
}
