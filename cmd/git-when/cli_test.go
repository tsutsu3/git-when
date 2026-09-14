package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tsutsu3/git-when/internal/axis"
	"github.com/tsutsu3/git-when/internal/collect"
	"github.com/tsutsu3/git-when/internal/render"
	"github.com/tsutsu3/git-when/internal/stats"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want options
	}{
		{"no args", nil, defaults(nil)},
		{"several dirs", []string{"src", "work"}, defaults(func(o *options) {
			o.roots = []string{"src", "work"}
		})},
		{
			"flags between dirs",
			[]string{"src", "--author", "alice@", "work", "--bots"},
			defaults(func(o *options) {
				o.roots = []string{"src", "work"}
				o.filter = collect.FilterOptions{Author: "alice@", Bots: true}
			}),
		},
		{"max depth", []string{"--max-depth=2", "src"}, defaults(func(o *options) {
			o.roots, o.maxDepth = []string{"src"}, 2
		})},
		{"unlimited depth", []string{"--max-depth", "0"}, defaults(func(o *options) {
			o.maxDepth = 0
		})},
		{"follow symlinks", []string{"--follow-symlinks"}, defaults(func(o *options) {
			o.followSymlinks = true
		})},
		{"default author", []string{"--default-author", "tsutsu3"}, defaults(func(o *options) {
			o.defaultAuthor = "tsutsu3"
		})},
		{"no github link", []string{"--no-github-link"}, defaults(func(o *options) {
			o.noGitHubLink = true
		})},
		{"no email", []string{"--no-email"}, defaults(func(o *options) {
			o.noEmail = true
		})},
		{"version", []string{"--version"}, defaults(func(o *options) {
			o.showVersion = true
		})},
		{"cache flags", []string{"--no-cache", "--cache-dir", "/tmp/c"}, defaults(func(o *options) {
			o.noCache, o.cacheDir = true, "/tmp/c"
		})},
		{"min total", []string{"--min-total", "50"}, defaults(func(o *options) {
			o.minTotal = 50
		})},
		{"view", []string{"--view", "week"}, defaults(func(o *options) {
			o.view, o.viewSet = "week", true
		})},
		{"pivot shorthand", []string{"-p", "year/hour"}, defaults(func(o *options) {
			o.pivot = "year/hour"
		})},
		{"csv to a file", []string{"-f", "csv", "-o", "rhythm.csv"}, defaults(func(o *options) {
			o.format, o.out, o.outSet = "csv", "rhythm.csv", true
		})},
		{
			"display options",
			[]string{"--week-start", "sun", "--weekend=fri,sat", "--scale", "log"},
			defaults(func(o *options) {
				o.weekStart, o.weekend, o.scale = "sun", "fri,sat", "log"
			}),
		},
		{"dash-dash ends flags", []string{"--", "-repo"}, defaults(func(o *options) {
			o.roots = []string{"-repo"}
		})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseArgs(c.args, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestParseArgsErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--no-such-flag"},
		{"--author"}, // No value.
		// In pflag a single dash starts short flags.
		// -bots means -b -o -t -s, and those are not defined.
		{"-bots"},
		{"--max-depth", "deep"},
	} {
		if got, err := parseArgs(args, io.Discard); err == nil {
			t.Errorf("parseArgs(%q) = %+v, want error", args, got)
		}
	}
}

func TestResolve(t *testing.T) {
	t.Setenv("COLORFGBG", "") // Fix the auto theme to a dark background.
	const sunday = 6
	friSat, err := stats.ParseWeekend("fri,sat")
	if err != nil {
		t.Fatal(err)
	}
	sqrt := render.Options{Scale: render.Sqrt}
	weekend := stats.DefaultWeekend()
	grid := func(name string, y, x axis.Kind) viewSel {
		return viewSel{name: name, kind: viewGrid, spec: axis.Spec{Y: y, X: x}}
	}
	heatmap := grid("heatmap", axis.Weekday, axis.Hour)
	hour := grid("hour", axis.None, axis.Hour)
	month := grid("month", axis.Year, axis.Month)
	week := viewSel{name: "week", kind: viewWeek}
	only := func(v viewSel) []viewSel { return []viewSel{v} }
	existingDir := t.TempDir()
	newDir := filepath.Join(t.TempDir(), "figures") + "/"

	cases := []struct {
		name string
		args []string
		want config
	}{
		{"defaults", nil, config{views: only(heatmap), weekend: weekend, render: sqrt}},
		{"pivot", []string{"-p", "year/hour"}, config{
			views: only(grid("year-hour", axis.Year, axis.Hour)), weekend: weekend, render: sqrt,
		}},
		{"explicit heatmap view", []string{"--view", "heatmap"}, config{
			views: only(heatmap), weekend: weekend, render: sqrt,
		}},
		{"week view", []string{"--view", "week", "--weekend", "fri,sat"}, config{
			views: only(week), weekend: friSat, render: sqrt,
		}},
		{"weekday view", []string{"--view", "weekday", "--week-start", "sun"}, config{
			views:     only(viewSel{name: "weekday", kind: viewWeekday}),
			weekStart: sunday, weekend: weekend, render: sqrt,
		}},
		{"days view", []string{"--view", "days"}, config{
			views: only(viewSel{name: "days", kind: viewDays}), weekend: weekend, render: sqrt,
		}},
		{"hour alias", []string{"--view", "hour"}, config{
			views: only(hour), weekend: weekend, render: sqrt,
		}},
		{"month alias", []string{"--view", "month"}, config{
			views: only(month), weekend: weekend, render: sqrt,
		}},
		{"several views in order", []string{"--view", " Week,heatmap "}, config{
			views: []viewSel{week, heatmap}, weekend: weekend, render: sqrt,
		}},
		{"min total", []string{"-p", "author/hour", "--min-total", "3"}, config{
			views: only(grid("author-hour", axis.Author, axis.Hour)), weekend: weekend,
			render: render.Options{Scale: render.Sqrt, MinRowTotal: 3},
		}},
		{
			"tzshift view",
			[]string{"--view", "tzshift"},
			config{
				views: only(
					viewSel{name: "tzshift", kind: viewTZShift},
				),
				weekend: weekend,
				render:  sqrt,
			},
		},
		{"csv to a file", []string{"--format", "CSV", "--out", "r.csv"}, config{
			format: formatCSV, out: "r.csv",
			views: only(heatmap), weekend: weekend, render: sqrt,
		}},
		// Formats other than term write to a default file without -o.
		// Writing them to stdout needs an explicit -o -.
		{"html to the default file", []string{"-f", "html"}, config{
			format: formatHTML, out: defaultHTMLName,
			views: only(heatmap), weekend: weekend, render: sqrt,
		}},
		{"csv to the default file", []string{"-f", "csv"}, config{
			format: formatCSV, out: defaultCSVName,
			views: only(heatmap), weekend: weekend, render: sqrt,
		}},
		{"svg to the default file", []string{"-f", "svg", "-v", "week"}, config{
			format: formatSVG, out: defaultSVGName,
			views: only(week), weekend: weekend, render: sqrt,
		}},
		{
			"svg split to the current directory",
			[]string{"-f", "svg", "--svg-layout", "split"},
			config{
				format: formatSVG, out: ".", split: true,
				views: only(heatmap), weekend: weekend, render: sqrt,
			},
		},
		{"html to stdout", []string{"-f", "html", "-o", "-"}, config{
			format: formatHTML, views: only(heatmap), weekend: weekend, render: sqrt,
		}},
		{"html to a file", []string{"-f", "html", "-o", "report.html"}, config{
			format: formatHTML, out: "report.html",
			views: only(heatmap), weekend: weekend, render: sqrt,
		}},
		// In HTML, --min-total hides authors with fewer commits from the author list.
		{"html with min total", []string{"-f", "html", "--min-total", "2"}, config{
			format: formatHTML, out: defaultHTMLName,
			views: only(heatmap), weekend: weekend,
			render: render.Options{Scale: render.Sqrt, MinRowTotal: 2},
		}},
		{
			"html with default author",
			[]string{"-f", "html", "--default-author", "^tsutsu3 "},
			config{
				format: formatHTML, out: defaultHTMLName,
				views: only(heatmap), weekend: weekend, render: sqrt,
				defaultAuthor: "^tsutsu3 ",
			},
		},
		{"html without the GitHub link", []string{"-f", "html", "--no-github-link"}, config{
			format: formatHTML, out: defaultHTMLName,
			views: only(heatmap), weekend: weekend, render: sqrt,
			noGitHubLink: true,
		}},
		{"html without emails", []string{"-f", "html", "--no-email"}, config{
			format: formatHTML, out: defaultHTMLName,
			views: only(heatmap), weekend: weekend, render: sqrt,
			noEmail: true,
		}},
		// The output decides between one file (sheet) and one file per view in a directory (split).
		{"svg sheet to stdout", []string{"-f", "svg", "-v", "all", "-o", "-"}, config{
			format: formatSVG, views: []viewSel{heatmap, hour, week, month},
			weekend: weekend, render: sqrt,
		}},
		{"svg sheet to a file", []string{"-f", "svg", "-v", "heatmap,week", "-o", "r.svg"}, config{
			format: formatSVG, out: "r.svg", views: []viewSel{heatmap, week},
			weekend: weekend, render: sqrt,
		}},
		{"svg split to a new directory", []string{"-f", "svg", "-o", newDir}, config{
			format: formatSVG, out: newDir, split: true, views: only(heatmap),
			weekend: weekend, render: sqrt,
		}},
		{"svg split to an existing directory", []string{"-f", "svg", "-o", existingDir}, config{
			format: formatSVG, out: existingDir, split: true, views: only(heatmap),
			weekend: weekend, render: sqrt,
		}},
		{"explicit layout", []string{"-f", "svg", "--svg-layout", "sheet", "-o", "r.svg"}, config{
			format: formatSVG, out: "r.svg", views: only(heatmap), weekend: weekend, render: sqrt,
		}},
		{
			"svg theme",
			[]string{"-f", "svg", "--theme", "light", "-o", "-"},
			config{
				format:  formatSVG,
				views:   only(heatmap),
				weekend: weekend,
				render: render.Options{
					Scale:    render.Sqrt,
					Theme:    render.Light,
					SVGTheme: render.SVGLight,
				},
			},
		},
		{"progress", []string{"--progress", "always"}, config{
			views: only(heatmap), weekend: weekend, render: sqrt, progress: true,
		}},
		{
			"summary view with options",
			[]string{"--view=summary", "--scale", "linear", "--color", "always"},
			config{
				views: only(viewSel{name: "summary", kind: viewSummary}), weekend: weekend,
				render: render.Options{Scale: render.Linear, Color: true},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mustResolve(t, c.args)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestResolveErrors(t *testing.T) {
	for _, args := range [][]string{
		// --pivot always draws a grid, so it conflicts with --view.
		{"--view", "week", "-p", "year/hour"},
		{"--view", "heatmap", "-p", "year/hour"},
		{"--view", "calendar"},
		{"-p", "hour"},
		{"--week-start", "someday"},
		{"--weekend", ""},
		{"--scale", "cubic"},
		{"--color", "sometimes"},
		{"--theme", "sepia"},
		{"--progress", "sometimes"},
		{"--timings", "sometimes"},
		{"--min-total", "-1"},
		// Only grids can hide rows.
		{"--view", "week", "--min-total", "5"},
		{"--no-cache", "--cache-dir", "x"},
		{"--format", "png"},
		{"-o", ""},
		{"-v", "heatmap,week,heatmap"},
		{"-v", "heatmap,"},
		{"-v", "week,heatmap", "--min-total", "2"},
		// Views that SVG cannot draw.
		{"-f", "svg", "-v", "summary"},
		{"-f", "svg", "-v", "heatmap,tzshift"},
		// Layouts that conflict with the output.
		{"-f", "svg", "--svg-layout", "split", "-o", "-"},
		{"-f", "svg", "--svg-layout", "split", "-o", "r.svg"},
		{"-f", "svg", "--svg-layout", "sheet", "-o", "out/"},
		{"-f", "svg", "--svg-layout", "grid"},
		{"--svg-layout", "sheet"},
		// The HTML page switches views itself.
		{"-f", "html", "-v", "week"},
		{"-f", "html", "-p", "year/hour"},
		{"-f", "html", "--svg-layout", "split", "-o", "out/"},
		// Only the HTML page has an author list to select from.
		{"--default-author", "tsutsu3"},
		{"-f", "csv", "--default-author", "tsutsu3"},
		{"-f", "html", "--default-author", "("},
		// Only the HTML page has the GitHub link.
		{"--no-github-link"},
		{"-f", "svg", "--no-github-link"},
		// Only the HTML report embeds author emails.
		{"--no-email"},
		{"-f", "csv", "--no-email"},
		{"-o", "out/"}, // Only svg writes into a directory.
		// CSV writes every bucket, so view options conflict with it.
		{"-f", "csv", "--view", "week"},
		{"-f", "csv", "--view", "heatmap"},
		{"-f", "csv", "-p", "year/hour"},
		{"-f", "csv", "--min-total", "2"},
	} {
		o, err := parseArgs(args, io.Discard)
		if err != nil {
			t.Fatalf("parseArgs(%q): %v", args, err)
		}
		if cfg, err := resolve(o, io.Discard, io.Discard); err == nil {
			t.Errorf("resolve(%q) = %+v, want error", args, cfg)
		}
	}
}

func TestUseColor(t *testing.T) {
	var buf bytes.Buffer
	for _, c := range []struct {
		when string
		want bool
	}{{"always", true}, {"never", false}, {"auto", false}} { // A buffer is not a terminal.
		if got, err := useColor(c.when, &buf); err != nil || got != c.want {
			t.Errorf("useColor(%q) = %v, %v; want %v", c.when, got, err, c.want)
		}
	}

	t.Setenv("NO_COLOR", "1")
	if got, _ := useColor("auto", os.Stdout); got {
		t.Error("useColor(auto) with NO_COLOR = true, want false")
	}
	if got, _ := useColor("always", os.Stdout); !got {
		t.Error("useColor(always) with NO_COLOR = false; an explicit request should win")
	}
}

func TestResolveTheme(t *testing.T) {
	for _, c := range []struct {
		flag, colorfgbg string
		want            render.Theme
	}{
		{"auto", "", render.Dark},      // Unknown means dark.
		{"auto", "15;0", render.Dark},  // White text on black.
		{"auto", "0;15", render.Light}, // Black text on white.
		{"auto", "0;default;7", render.Light},
		{"light", "15;0", render.Light}, // An explicit theme wins.
		{"dark", "0;15", render.Dark},
	} {
		t.Setenv("COLORFGBG", c.colorfgbg)
		if got, err := resolveTheme(c.flag); err != nil || got != c.want {
			t.Errorf("--theme %s with COLORFGBG=%q: got %v, %v; want %v",
				c.flag, c.colorfgbg, got, err, c.want)
		}
	}
}

func TestCacheDirFor(t *testing.T) {
	base, err := defaultCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, filepath.Join(base, "git-when")},
		{[]string{"--cache-dir", "/var/cache/gw"}, "/var/cache/gw"},
		{[]string{"--no-cache"}, ""},
	} {
		o, err := parseArgs(c.args, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if got := cacheDirFor(o); got != c.want {
			t.Errorf("cacheDirFor(%q) = %q, want %q", c.args, got, c.want)
		}
	}

	// Without a cache location, git-when runs without a cache instead of failing.
	orig := defaultCacheDir
	t.Cleanup(func() { defaultCacheDir = orig })
	defaultCacheDir = func() (string, error) { return "", errors.New("no home directory") }
	if got := cacheDirFor(defaults(nil)); got != "" {
		t.Errorf("cacheDirFor without a cache directory = %q, want empty", got)
	}
}
