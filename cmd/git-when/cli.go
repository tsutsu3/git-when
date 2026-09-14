package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/pflag"

	"github.com/tsutsu3/git-when/internal/axis"
	"github.com/tsutsu3/git-when/internal/collect"
	"github.com/tsutsu3/git-when/internal/render"
	"github.com/tsutsu3/git-when/internal/stats"
)

// defaultMaxDepth is the default search depth.
// It keeps a walk from a home directory from going too deep.
const defaultMaxDepth = 5

// defaultCacheDir returns the parent directory of the cache.
// Tests replace it with a temporary directory.
var defaultCacheDir = os.UserCacheDir

// options holds the raw command-line values.
// resolve interprets and validates them.
type options struct {
	roots          []string
	format         string
	out            string
	outSet         bool // Whether -o was given.
	svgLayout      string
	maxDepth       int
	followSymlinks bool
	filter         collect.FilterOptions
	defaultAuthor  string
	view           string
	viewSet        bool // Whether --view was given.
	pivot          string
	weekStart      string
	weekend        string
	scale          string
	color          string
	theme          string
	progress       string
	minTotal       int
	noCache        bool
	noGitHubLink   bool
	noEmail        bool
	showVersion    bool
	cacheDir       string
	timings        string
}

// parseArgs reads the flags and the positional arguments.
// Positional arguments are the directories to search.
func parseArgs(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := pflag.NewFlagSet("git-when", pflag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVarP(&o.view, "view", "v", "heatmap",
		"`names` of views, comma-separated, or all:\n"+
			"heatmap, hour, month, week, weekday, days, summary, tzshift")
	fs.StringVarP(&o.pivot, "pivot", "p", "",
		"draw a grid with `y/x` axes, e.g. year/hour or /hour\n"+
			"(axes: hour, wday, month, year, project, author; heatmap = wday/hour)")
	fs.StringVarP(&o.format, "format", "f", "term",
		"output `format`: term, csv (every bucket, one row each), svg,\n"+
			"html (a single page to switch repositories, years and views)")
	fs.StringVarP(&o.out, "out", "o", "-",
		"write the output to `file` instead of stdout (-: stdout)\n"+
			"(default: stdout for term; "+defaultCSVName+", "+defaultSVGName+" or "+defaultHTMLName+
			" for the others,\nor the current directory for svg split)")
	fs.StringVar(&o.svgLayout, "svg-layout", "",
		"`layout` of svg: sheet (all views in one file) or split (one file per view in -o dir/)\n"+
			"(default: split when -o is a directory, otherwise sheet)")
	fs.StringVar(&o.weekStart, "week-start", "mon",
		"first `day` of the week in grids and the weekday and days views")
	fs.StringVar(&o.weekend, "weekend", "sat,sun",
		"weekend `days` for the week, weekday, days and summary views")
	fs.StringVar(&o.scale, "scale", "sqrt",
		"shading `scale` of grids: sqrt, linear, log")
	fs.StringVar(&o.color, "color", "never",
		"use colors: `when` = never, auto, always")
	fs.StringVar(&o.theme, "theme", "auto",
		"background for heatmap colors: `name` = auto, dark, light\n"+
			"(auto: terminal from COLORFGBG; svg follows the viewer's color scheme)")
	fs.StringVar(&o.progress, "progress", "auto",
		"show progress on stderr: `when` = auto (only on a terminal), always, never")
	fs.StringVar(&o.timings, "timings", "auto",
		"show how long each step took on stderr: `when` = auto (only on a terminal), always, never")
	fs.StringVar(&o.filter.Author, "author", "",
		"count only commits whose \"Name <email>\" matches `regexp`")
	fs.StringVar(&o.filter.ExcludeAuthor, "exclude-author", "",
		"skip commits whose \"Name <email>\" matches `regexp`")
	fs.StringVar(&o.defaultAuthor, "default-author", "",
		"html: select the author whose \"Name <email>\" matches `regexp` when the page opens\n"+
			"(the match with the most commits)")
	fs.BoolVar(&o.noGitHubLink, "no-github-link", false,
		"html: leave out the link to the git-when repository on GitHub")
	fs.BoolVar(&o.noEmail, "no-email", false,
		"html: leave out author emails (authors with the same name are numbered)")
	fs.BoolVar(&o.showVersion, "version", false, "print the version and exit")
	fs.BoolVar(&o.filter.Bots, "bots", false,
		"include commits by bots (excluded by default)")
	fs.IntVar(&o.maxDepth, "max-depth", defaultMaxDepth,
		"search at most `n` directory levels below each dir (0: no limit)")
	fs.BoolVar(&o.followSymlinks, "follow-symlinks", false,
		"also search symbolic links to directories\n"+
			"(links into the dir or back to a searched directory are skipped)")
	fs.IntVar(&o.minTotal, "min-total", 0,
		"in grids, hide rows with fewer than `n` commits in total (0: show all)")
	fs.BoolVar(&o.noCache, "no-cache", false,
		"do not read or write the git log cache")
	fs.StringVar(&o.cacheDir, "cache-dir", "",
		"keep the git log cache in `dir` (default: the user cache directory + /git-when)")
	fs.SortFlags = false
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: git-when [flags] [dir...]")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return o, err
	}
	o.viewSet = fs.Changed("view")
	o.outSet = fs.Changed("out")
	o.roots = fs.Args()
	if len(o.roots) == 0 {
		o.roots = []string{"."}
	}
	return o, nil
}

type formatKind int

const (
	formatTerm formatKind = iota
	formatCSV
	formatSVG
	formatHTML
)

// config is the validated configuration.
type config struct {
	format    formatKind
	out       string    // Output file. Empty means stdout. A directory when split is true.
	split     bool      // Whether SVG views go to separate files.
	views     []viewSel // Views in drawing order.
	weekStart int
	weekend   stats.Weekend
	render    render.Options
	progress  bool // Whether to show progress on stderr.
	timings   bool // Whether to show phase timings on stderr.
	// defaultAuthor is the --default-author regular expression. It is resolved after collection.
	defaultAuthor string
	// noEmail is --no-email. The HTML report then has no author emails.
	noEmail bool
	// noGitHubLink is --no-github-link. The HTML report then has no link to the repository.
	noGitHubLink bool
}

// resolve interprets the options.
// Conflicting options are errors. resolve never guesses what the user meant.
// stdout and stderr decide whether colors and progress may be used.
func resolve(o options, stdout, stderr io.Writer) (config, error) {
	var cfg config
	var err error

	switch strings.ToLower(strings.TrimSpace(o.format)) {
	case "term":
		cfg.format = formatTerm
	case "csv":
		cfg.format = formatCSV
		// CSV writes every bucket, so view options have no effect.
		// Report them instead of ignoring them.
		if o.viewSet || o.pivot != "" || o.minTotal != 0 {
			return cfg, errors.New(
				"--view, --pivot and --min-total do not apply to --format csv (it writes every bucket)",
			)
		}
	case "svg":
		cfg.format = formatSVG
	case "html":
		cfg.format = formatHTML
		// The HTML page switches views itself, so view options have no effect.
		// --min-total still applies. The page hides authors with fewer commits from its author list.
		if o.viewSet || o.pivot != "" {
			return cfg, errors.New(
				"--view and --pivot do not apply to --format html (switch views in the page)",
			)
		}
	default:
		return cfg, fmt.Errorf("--format: unknown format %q (want term, csv, svg, html)", o.format)
	}

	switch {
	case o.pivot != "" && o.viewSet:
		return cfg, errors.New(
			"--view and --pivot cannot be used together (--pivot always draws a grid)",
		)
	case o.pivot != "":
		spec, perr := axis.ParseSpec(o.pivot)
		if perr != nil {
			return cfg, fmt.Errorf("--pivot: %w", perr)
		}
		cfg.views = []viewSel{{name: pivotName(o.pivot), kind: viewGrid, spec: spec}}
	default:
		if cfg.views, err = parseViews(o.view, cfg.format); err != nil {
			return cfg, fmt.Errorf("--view: %w", err)
		}
	}
	if cfg.out, cfg.split, err = resolveOut(o, cfg.format); err != nil {
		return cfg, err
	}

	if cfg.weekStart, err = axis.ParseWeekday(o.weekStart); err != nil {
		return cfg, fmt.Errorf("--week-start: %w", err)
	}
	if cfg.weekend, err = stats.ParseWeekend(o.weekend); err != nil {
		return cfg, fmt.Errorf("--weekend: %w", err)
	}
	if cfg.render.Scale, err = render.ParseScale(o.scale); err != nil {
		return cfg, fmt.Errorf("--scale: %w", err)
	}
	colorOut := stdout
	if cfg.out != "" {
		colorOut = nil // A file is not a terminal, so auto never adds colors.
	}
	if cfg.render.Color, err = useColor(o.color, colorOut); err != nil {
		return cfg, fmt.Errorf("--color: %w", err)
	}
	if cfg.render.Theme, err = resolveTheme(o.theme); err != nil {
		return cfg, fmt.Errorf("--theme: %w", err)
	}
	if cfg.render.SVGTheme, err = render.ParseSVGTheme(o.theme); err != nil {
		return cfg, fmt.Errorf("--theme: %w", err)
	}
	if cfg.progress, err = wantOnTerminal(o.progress, stderr); err != nil {
		return cfg, fmt.Errorf("--progress: %w", err)
	}
	if cfg.timings, err = wantOnTerminal(o.timings, stderr); err != nil {
		return cfg, fmt.Errorf("--timings: %w", err)
	}
	switch {
	case o.minTotal < 0:
		return cfg, fmt.Errorf("--min-total: must be 0 or more, got %d", o.minTotal)
	case o.minTotal > 0 && cfg.format != formatHTML && slices.ContainsFunc(cfg.views,
		func(v viewSel) bool { return v.kind != viewGrid }):
		return cfg, errors.New(
			"--min-total applies only to grids (heatmap, hour, month, --pivot) and --format html",
		)
	}
	cfg.render.MinRowTotal = o.minTotal
	if o.defaultAuthor != "" {
		if cfg.format != formatHTML {
			return cfg, errors.New("--default-author applies only to --format html")
		}
		// The authors are known only after collection. A bad pattern is still an argument error.
		if _, err := regexp.Compile(o.defaultAuthor); err != nil {
			return cfg, fmt.Errorf("--default-author: %w", err)
		}
		cfg.defaultAuthor = o.defaultAuthor
	}
	if o.noGitHubLink && cfg.format != formatHTML {
		return cfg, errors.New("--no-github-link applies only to --format html")
	}
	cfg.noGitHubLink = o.noGitHubLink
	if o.noEmail && cfg.format != formatHTML {
		return cfg, errors.New("--no-email applies only to --format html")
	}
	cfg.noEmail = o.noEmail
	if o.noCache && o.cacheDir != "" {
		return cfg, errors.New("--no-cache and --cache-dir cannot be used together")
	}
	return cfg, nil
}

// useColor decides from --color and the output whether to use ANSI colors.
// With auto, colors are used only on a terminal and never when NO_COLOR is set.
func useColor(when string, stdout io.Writer) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(when)) {
	case "always":
		return true, nil
	case "never":
		return false, nil
	case "auto":
		if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
			return false, nil
		}
		return isTerminal(stdout), nil
	}
	return false, fmt.Errorf("unknown value %q (want never, auto, always)", when)
}

// wantOnTerminal decides from --progress or --timings whether to write extra lines to stderr.
// With auto, they are written only on a terminal.
// This keeps control characters and extra lines out of pipes and files.
func wantOnTerminal(when string, stderr io.Writer) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(when)) {
	case "always":
		return true, nil
	case "never":
		return false, nil
	case "auto":
		return os.Getenv("TERM") != "dumb" && isTerminal(stderr), nil
	}
	return false, fmt.Errorf("unknown value %q (want auto, always, never)", when)
}

// isTerminal reports whether w is a terminal (a character device).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// resolveTheme reads --theme.
// With auto, it uses the background color in COLORFGBG ("fg;bg").
// If that is unknown, it assumes a dark background, which most terminals use.
func resolveTheme(s string) (render.Theme, error) {
	if strings.ToLower(strings.TrimSpace(s)) != "auto" {
		return render.ParseTheme(s)
	}
	fields := strings.Split(os.Getenv("COLORFGBG"), ";")
	switch fields[len(fields)-1] {
	case "7", "15":
		return render.Light, nil
	}
	return render.Dark, nil
}

// cacheDirFor returns the git log cache directory. Empty means no cache.
// If no location can be found (for example without HOME), git-when runs without a cache.
func cacheDirFor(o options) string {
	switch {
	case o.noCache:
		return ""
	case o.cacheDir != "":
		return o.cacheDir
	}
	base, err := defaultCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "git-when")
}
