package progress

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/tsutsu3/git-when/internal/render"
)

// fakeClock lets a test move the Reporter clock by hand.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestReporterThrottlesUpdate(t *testing.T) {
	var buf bytes.Buffer
	clock := &fakeClock{t: time.Unix(0, 0)}
	r := newReporter(&buf, clock)

	r.Update("dirs %d", 1) // The first update is always shown.
	clock.advance(50 * time.Millisecond)
	r.Update("dirs %d", 2) // Too soon, so nothing is shown.
	clock.advance(60 * time.Millisecond)
	r.Update("dirs %d", 3) // 110ms after the last redraw.
	r.Set("repo %d", 1)    // Set always redraws.
	r.Set("repo %d", 2)
	r.Clear()
	r.Clear() // Nothing is on screen, so nothing is written.

	want := clearLine + "dirs 1" + clearLine + "dirs 3" +
		clearLine + "repo 1" + clearLine + "repo 2" + clearLine
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestReporterDisabled(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, false)
	r.Update("a")
	r.Set("b")
	r.Clear()
	if buf.Len() != 0 {
		t.Errorf("disabled reporter wrote %q", buf.String())
	}
}

func TestReporterKeepsOneLine(t *testing.T) {
	var buf bytes.Buffer
	r := newReporter(&buf, &fakeClock{})
	r.Set("%s", strings.Repeat("長い", 100))

	line := strings.TrimPrefix(buf.String(), clearLine)
	if strings.Contains(line, "\n") || render.Width(line) > maxWidth {
		t.Errorf("line width %d > %d or has a newline: %q", render.Width(line), maxWidth, line)
	}
}

func TestTruncateLeft(t *testing.T) {
	for _, c := range []struct {
		in   string
		w    int
		want string
	}{
		{"/home/tt/src", 20, "/home/tt/src"},
		{"/home/tt/Workspace/long/path", 11, "…/long/path"},
		{"/ホーム/作業/リポジトリ", 12, "…/リポジトリ"}, // Wide characters count as 2.
		{"abc", 0, ""},
	} {
		got := TruncateLeft(c.in, c.w)
		if got != c.want {
			t.Errorf("TruncateLeft(%q, %d) = %q, want %q", c.in, c.w, got, c.want)
		}
		if render.Width(got) > c.w {
			t.Errorf("TruncateLeft(%q, %d) is %d wide", c.in, c.w, render.Width(got))
		}
	}
}

func newReporter(buf *bytes.Buffer, c *fakeClock) *Reporter {
	r := New(buf, true)
	r.now = c.now
	return r
}
