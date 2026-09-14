package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tsutsu3/git-when/internal/gittest"
	"github.com/tsutsu3/git-when/internal/model"
)

// TestRun checks a whole run from discovery to terminal output.
func TestRun(t *testing.T) {
	root := fixture(t)

	out, errOut := runOK(t, root)
	for _, want := range []string{
		"2 commits in 2 repositories (author local time)",
		"Mon ", "Tue ",
		"sqrt scale, max 1 at Mon 09", // Both commits are at 09 in author local time.
	} {
		if !strings.Contains(out, want) {
			t.Errorf("heatmap lacks %q:\n%s", want, out)
		}
	}
	wantWarning := "warning: skipping " + filepath.Join(root, "broken")
	if !strings.Contains(errOut, wantWarning) {
		t.Errorf("stderr lacks %q:\n%s", wantWarning, errOut)
	}
	// stderr is not a terminal, so the auto default shows no progress.
	if strings.Contains(errOut, "\r") {
		t.Errorf("progress written to a non-terminal stderr: %q", errOut)
	}
	if strings.Contains(errOut, "timings:") {
		t.Errorf("timings written to a non-terminal stderr: %q", errOut)
	}

	views := []struct {
		args []string
		want []string
	}{
		{[]string{"--view", "summary"}, []string{"app", "empty", "total"}},
		{[]string{"--view", "week"}, []string{"weekdays (5 days)", "weekend index 0.00"}},
		{[]string{"--view", "weekday"}, []string{"Mon  ", "Tue  ", "weekend"}},
		{[]string{"--view", "days"}, []string{"Mon ", "Sat* ", "Sun*", "max 1 at Mon 09"}},
		{[]string{"--view", "hour"}, []string{"05│06", "max 2 at 09"}},
		{[]string{"--view", "month"}, []string{"2024 ", "max 2 at 2024 3"}},
		{
			[]string{"--view", "tzshift"},
			[]string{"year  -08:00  +09:00  total", "2024 ", "-08:00  50%"},
		},
		{
			[]string{"-p", "project/hour", "--min-total", "1"},
			[]string{"app ", "1 of 2 project rows hidden"},
		},
		{[]string{"-p", "year/hour"}, []string{"2024 "}},
		{
			[]string{"-v", "heatmap,week"},
			[]string{"== Weekday × hour ==", "== Weekdays vs weekend by hour ==", "weekend index"},
		},
		{[]string{"--week-start", "sun"}, []string{"at Mon 09"}},
	}
	for _, v := range views {
		out, _ := runOK(t, append(v.args, root)...)
		for _, want := range v.want {
			if !strings.Contains(out, want) {
				t.Errorf("%q output lacks %q:\n%s", v.args, want, out)
			}
		}
		if strings.Contains(out, "dep") {
			t.Errorf("%q output includes a repository under node_modules:\n%s", v.args, out)
		}
	}
}

func TestRunVersion(t *testing.T) {
	out, errOut := runOK(t, "--version")
	if out != version+"\n" {
		t.Errorf("stdout = %q, want %q", out, version+"\n")
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty", errOut)
	}
}

func TestRunMultipleRoots(t *testing.T) {
	work, home := t.TempDir(), t.TempDir()
	place(t, gittest.New(t, gittest.CommitAt("2024-03-05T09:00:00+09:00")), work, "app")
	place(t, gittest.New(t, gittest.CommitAt("2024-03-09T22:00:00+09:00")), home, "app")

	out, _ := runOK(t, "--view", "summary", work, home)
	// Repositories with the same name are told apart by the root's base name.
	for _, want := range []string{
		"2 commits in 2 repositories",
		filepath.Base(work) + "/app ",
		filepath.Base(home) + "/app ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestRunDefaultMaxDepth(t *testing.T) {
	root := t.TempDir()
	place(t, gittest.New(t, gittest.CommitAt("2024-03-05T09:00:00+09:00")), root, "1/2/3/4/five")
	place(t, gittest.New(t, gittest.CommitAt("2024-03-05T10:00:00+09:00")), root, "1/2/3/4/5/six")

	out, _ := runOK(t, "--view", "summary", root)
	if !strings.Contains(out, "1/2/3/4/five") || strings.Contains(out, "six") {
		t.Errorf("default depth %d should find depth 5 only:\n%s", defaultMaxDepth, out)
	}
	out, _ = runOK(t, "--view", "summary", "--max-depth", "0", root)
	if !strings.Contains(out, "1/2/3/4/5/six") {
		t.Errorf("--max-depth 0 should search without limit:\n%s", out)
	}
}

func TestRunErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		code int
		msg  string
	}{
		{"no repositories", []string{t.TempDir()}, exitError, "no git repositories found"},
		{"missing dir", []string{filepath.Join(t.TempDir(), "missing")}, exitError, "missing"},
		{
			"invalid regexp",
			[]string{"--author", "(", t.TempDir()},
			exitUsage,
			"invalid regular expression",
		},
		{"unknown flag", []string{"--no-such-flag"}, exitUsage, "no-such-flag"},
		{
			"conflicting view and pivot",
			[]string{"--view", "week", "-p", "year/hour", t.TempDir()},
			exitUsage,
			"cannot be used together",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(t.Context(), c.args, &stdout, &stderr); code != c.code {
				t.Errorf("exit code = %d, want %d", code, c.code)
			}
			if !strings.Contains(stderr.String(), c.msg) {
				t.Errorf("stderr lacks %q:\n%s", c.msg, stderr.String())
			}
		})
	}
}

func TestRunProgress(t *testing.T) {
	root := fixture(t)
	out, errOut := runOK(t, "--progress", "always", root)

	const clearLine = "\r\x1b[K"
	for _, want := range []string{
		clearLine + "discovering: ",
		clearLine + "reading git log: 1/3  app",
		// A warning clears the progress line and starts a new line.
		clearLine + "warning: skipping " + filepath.Join(root, "broken"),
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr lacks %q:\n%q", want, errOut)
		}
	}
	// The progress line is cleared at the end.
	if !strings.HasSuffix(errOut, clearLine) {
		t.Errorf("progress line is left on the screen: %q", errOut)
	}
	if strings.ContainsAny(out, "\r\x1b") {
		t.Errorf("progress leaked into stdout: %q", out)
	}
}

func TestRunCache(t *testing.T) {
	root := fixture(t)
	dir := filepath.Join(t.TempDir(), "cache")

	first, _ := runOK(t, "--view", "summary", "--cache-dir", dir, root)
	// Only app is cached.
	// The empty repository has no HEAD, and the broken repository cannot be read.
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("cache dir has %v (%v), want 1 entry", entries, err)
	}
	second, _ := runOK(t, "--view", "summary", "--cache-dir", dir, root)
	if second != first {
		t.Errorf("output with the cache differs:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestRunTimings(t *testing.T) {
	root := fixture(t)
	out, errOut := runOK(t, "--timings", "always", "--cache-dir", t.TempDir(), root)

	// Four directories are visited: root, app, broken, and empty. node_modules is skipped.
	// Only app misses the cache and runs git log.
	// empty has no HEAD, so it is never cached.
	for _, want := range []string{
		"\ntimings: total ",
		"  discover ",
		"4 dirs, 3 repositories",
		"  read logs ",
		"(0 hits, 1 misses)",
		"  render ",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr lacks %q:\n%s", want, errOut)
		}
	}
	if strings.Contains(out, "timings") {
		t.Errorf("timings leaked into stdout:\n%s", out)
	}

	_, errOut = runOK(t, "--timings", "always", "--no-cache", root)
	if !strings.Contains(errOut, "cache off") {
		t.Errorf("stderr lacks %q with --no-cache:\n%s", "cache off", errOut)
	}
}

func TestDefaultAuthorIndex(t *testing.T) {
	agg := model.New()
	agg.Projects = []model.Project{{Name: "app"}}
	tm := time.Date(2024, 3, 5, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		author  model.Author
		commits int
	}{
		{model.Author{Name: "tsutsu3", Email: "old@example.com"}, 1},
		{model.Author{Name: "tsutsu3", Email: "new@example.com"}, 3},
		{model.Author{Name: "alice", Email: "alice@example.com"}, 2},
	} {
		i, err := agg.AuthorIndex(c.author)
		if err != nil {
			t.Fatal(err)
		}
		for range c.commits {
			if err := agg.Add(i, 0, tm, 0); err != nil {
				t.Fatal(err)
			}
		}
	}

	for _, c := range []struct {
		expr string
		want int
	}{
		{"tsutsu3", 1}, // Two identities match. The one with more commits wins.
		{"old@", 0},
		{"^alice <", 2},
	} {
		got, err := defaultAuthorIndex(agg, c.expr)
		if err != nil || got != c.want {
			t.Errorf("defaultAuthorIndex(%q) = %d, %v, want %d", c.expr, got, err, c.want)
		}
	}
	if _, err := defaultAuthorIndex(agg, "bob"); err == nil {
		t.Error("defaultAuthorIndex(bob) succeeded without a matching author")
	}
}

func TestCommandLine(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "git-when"},
		{[]string{"-f", "csv", "--weekend=sat,sun", "/home/me/src"}, "git-when -f csv --weekend=sat,sun /home/me/src"},
		{[]string{"~/my src", "--author", "it's"}, `git-when '~/my src' --author 'it'\''s'`},
		{[]string{"", "a*b", "作業"}, `git-when '' 'a*b' '作業'`},
	} {
		if got := commandLine(c.args); got != c.want {
			t.Errorf("commandLine(%q) = %s, want %s", c.args, got, c.want)
		}
	}
}

func TestFmtDuration(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{0, "<1ms"},
		{500 * time.Microsecond, "<1ms"},
		{37 * time.Millisecond, "37ms"},
		{999 * time.Millisecond, "999ms"},
		{1234 * time.Millisecond, "1.23s"},
		{65 * time.Second, "65.00s"},
	} {
		if got := fmtDuration(c.d); got != c.want {
			t.Errorf("fmtDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
