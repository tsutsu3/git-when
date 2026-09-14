package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tsutsu3/git-when/internal/gittest"
)

func TestRunCSV(t *testing.T) {
	root := fixture(t)
	out, _ := runOK(t, "-f", "csv", "-o", "-", root)

	app := filepath.Join(root, "app")
	head := strings.TrimSpace(gittest.Run(t, app, nil, "rev-parse", "HEAD"))
	for _, want := range []string{
		"# git-when " + version + "\n",
		"# command: git-when -f csv -o - " + shellQuote(root) + "\n",
		"# period: 2024-03 to 2024-03\n",
		"# repository: head=" + head + " commits=2 name=app\n",
		"# repository: head=- commits=0 name=empty\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("csv lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "commits in") {
		t.Errorf("csv includes the terminal header:\n%s", out)
	}

	// With the # lines skipped, the output is plain CSV.
	r := csv.NewReader(strings.NewReader(out))
	r.Comment = '#'
	records, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	name, email := gittest.DefaultName, gittest.DefaultEmail
	want := [][]string{
		{"project", "author_name", "author_email", "year", "month", "weekday", "hour", "commits"},
		{"app", name, email, "2024", "3", "1", "9", "1"}, // Monday 09:00 (-08:00)
		{"app", name, email, "2024", "3", "2", "9", "1"}, // Tuesday 09:00 (+09:00)
	}
	if !reflect.DeepEqual(records, want) {
		t.Errorf("records = %q\nwant      %q", records, want)
	}

	// With -o the output goes to the file and stdout stays empty.
	// Only the recorded command line differs.
	path := filepath.Join(t.TempDir(), "rhythm.csv")
	stdout, _ := runOK(t, "-f", "csv", "-o", path, root)
	if stdout != "" {
		t.Errorf("stdout with -o = %q, want empty", stdout)
	}
	data, err := os.ReadFile(path) //nolint:gosec // A file in the test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Replace(string(data), " -o "+shellQuote(path), " -o -", 1); got != out {
		t.Errorf("file differs from stdout:\nfile:\n%s\nstdout:\n%s", data, out)
	}
}

func TestRunOut(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(t.TempDir(), "summary.txt")
	runOK(t, "--view", "summary", "-o", path, root)
	data, err := os.ReadFile(path) //nolint:gosec // A file in the test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "2 commits in 2 repositories") {
		t.Errorf("-o with the terminal format wrote:\n%s", data)
	}

	// A run that fails because there are no repositories creates no file.
	missing := filepath.Join(t.TempDir(), "never.csv")
	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), []string{"-f", "csv", "-o", missing, t.TempDir()},
		&stdout, &stderr); code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("failed run left %s (stat: %v)", missing, err)
	}

	// CSV has the header row even with no commits.
	empty := t.TempDir()
	place(t, gittest.New(t), empty, "empty")
	out, _ := runOK(t, "-f", "csv", "-o", "-", empty)
	if !strings.HasSuffix(
		out,
		"\nproject,author_name,author_email,year,month,weekday,hour,commits\n",
	) {
		t.Errorf("csv without commits:\n%s", out)
	}
}

func TestRunSVG(t *testing.T) {
	root := fixture(t)

	// Several views on stdout are stacked in one sheet.
	out, _ := runOK(t, "-f", "svg", "-v", "heatmap,week", "--no-cache", "-o", "-", root)
	wellFormed(t, "stdout", out)
	for _, want := range []string{
		`<title>Mon 09: 1 commit</title>`,
		`>Weekday × hour</text>`,
		`>Weekdays vs weekend by hour</text>`,
		// A command line with "--" stays intact inside <metadata>.
		"command: git-when -f svg -v heatmap,week --no-cache -o - " + shellQuote(root) + "\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("svg lacks %q:\n%s", want, out)
		}
	}

	// A directory output writes one file per view (split).
	dir := filepath.Join(t.TempDir(), "figures") + "/"
	if stdout, _ := runOK(t, "-f", "svg", "-v", "all", "-o", dir, root); stdout != "" {
		t.Errorf("stdout with -o dir/ = %q, want empty", stdout)
	}
	titles := map[string]string{
		"git-when.all.heatmap.svg": "Weekday × hour",
		"git-when.all.hour.svg":    "Commits by hour",
		"git-when.all.month.svg":   "Year × month",
		"git-when.all.week.svg":    "Weekdays vs weekend by hour",
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != len(titles) {
		t.Fatalf("%s has %v (%v), want %d files", dir, entries, err, len(titles))
	}
	for name, title := range titles {
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // A test temp dir.
		if err != nil {
			t.Fatal(err)
		}
		wellFormed(t, name, string(data))
		// Each file has only its own view.
		for _, other := range titles {
			if got := strings.Contains(string(data), ">"+other+"</text>"); got != (other == title) {
				t.Errorf("%s: has heading %q = %v", name, other, got)
			}
		}
	}

	// A file output writes several views into one file.
	path := filepath.Join(t.TempDir(), "rhythm.svg")
	runOK(t, "-f", "svg", "-v", "hour,month", "-o", path, root)
	data, err := os.ReadFile(path) //nolint:gosec // The test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	if s := string(data); !strings.Contains(s, ">Commits by hour</text>") ||
		!strings.Contains(s, ">Year × month</text>") {
		t.Errorf("sheet file lacks a view:\n%s", s)
	}
}

func TestRunHTML(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(t.TempDir(), "report.html")
	runOK(t, "-f", "html", "--weekend", "fri,sat", "--week-start", "sun",
		"--default-author", gittest.DefaultName, "-o", path, root)
	data, err := os.ReadFile(path) //nolint:gosec // The test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{
		"<script>window.__GIT_WHEN_DATA__={",
		`"name":"app"`,
		`"period":"2024-03 to 2024-03"`,
		// The browser uses the weekend and week start, so they are in the data.
		`"weekStart":6,"weekend":[4,5]`,
		// The only author matches --default-author.
		`"defaultAuthor":0`,
		// Local paths are left out of the published data.
		`"path":""`,
		`"roots":[]`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("html lacks %q", want)
		}
	}
	if strings.Contains(out, "generateSample") {
		t.Error("html still has the sample generator")
	}
	// The GitHub link is shown by default, so the data has no flag to hide it.
	// The application script also names the field, so check the JSON value.
	if strings.Contains(out, `"noGitHubLink":true`) {
		t.Error("html hides the GitHub link without --no-github-link")
	}
	hidden, _ := runOK(t, "-f", "html", "-o", "-", "--no-github-link", root)
	if !strings.Contains(hidden, `"noGitHubLink":true`) {
		t.Error("html lacks noGitHubLink with --no-github-link")
	}
	// Emails are in the report by default and left out with --no-email.
	if !strings.Contains(out, gittest.DefaultEmail) {
		t.Error("html lacks the author email without --no-email")
	}
	noEmail, _ := runOK(t, "-f", "html", "-o", "-", "--no-email", root)
	if strings.Contains(noEmail, gittest.DefaultEmail) || !strings.Contains(noEmail, `"email":""`) {
		t.Error("html keeps the author email with --no-email")
	}
}

// TestRunDefaultOut checks that formats other than term write to a default file
// in the current directory without -o, and report the path on stderr.
func TestRunDefaultOut(t *testing.T) {
	root := fixture(t)
	t.Chdir(t.TempDir())
	for _, c := range []struct {
		args  []string
		files []string
		wrote string
	}{
		{[]string{"-f", "csv"}, []string{defaultCSVName}, "wrote " + defaultCSVName},
		{[]string{"-f", "svg", "-v", "heatmap,week"}, []string{defaultSVGName}, "wrote " + defaultSVGName},
		{[]string{"-f", "html"}, []string{defaultHTMLName}, "wrote " + defaultHTMLName},
		// Split file names have a prefix, so they go to the current directory.
		{
			[]string{"-f", "svg", "-v", "hour,week", "--svg-layout", "split"},
			[]string{"git-when.all.hour.svg", "git-when.all.week.svg"},
			"wrote 2 files to .",
		},
	} {
		stdout, stderr := runOK(t, append(c.args, root)...)
		if stdout != "" {
			t.Errorf("%q: stdout = %q, want empty", c.args, stdout)
		}
		if !strings.Contains(stderr, c.wrote+"\n") {
			t.Errorf("%q: stderr lacks %q:\n%s", c.args, c.wrote, stderr)
		}
		for _, f := range c.files {
			if _, err := os.Stat(f); err != nil {
				t.Errorf("%q: %v", c.args, err)
			}
		}
	}
}
