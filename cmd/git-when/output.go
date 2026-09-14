package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tsutsu3/git-when/internal/axis"
	"github.com/tsutsu3/git-when/internal/model"
	"github.com/tsutsu3/git-when/internal/render"
	"github.com/tsutsu3/git-when/internal/stats"
)

// Default output files for formats other than term when -o is omitted.
// These formats are hard to read in a terminal, so they go to a file.
const (
	defaultCSVName  = "git-when.csv"
	defaultSVGName  = "git-when.svg"
	defaultHTMLName = "git-when.html"
)

// maxTZColumns is the number of UTC offsets shown as columns in the tzshift view.
// The rest are grouped as other.
const maxTZColumns = 8

// defaultOutNames maps each format to its default output file.
// term is not listed because it writes to stdout.
var defaultOutNames = map[formatKind]string{
	formatCSV:  defaultCSVName,
	formatSVG:  defaultSVGName,
	formatHTML: defaultHTMLName,
}

// termViews and svgViews are the views that --view all selects for each format.
var (
	termViews = []string{
		"heatmap",
		"hour",
		"month",
		"week",
		"weekday",
		"days",
		"summary",
		"tzshift",
	}
	svgViews = []string{"heatmap", "hour", "week", "month"}
)

type viewKind int

const (
	viewGrid viewKind = iota
	viewWeek
	viewWeekday
	viewDays
	viewSummary
	viewTZShift
)

// viewSel is one view to draw.
type viewSel struct {
	name string // The --view name. Split SVG file names use it too.
	kind viewKind
	spec axis.Spec // Axes for viewGrid.
}

func parseView(name string) (viewSel, error) {
	v := viewSel{name: strings.ToLower(strings.TrimSpace(name))}
	var err error
	switch v.name {
	case "heatmap":
		v.spec, err = axis.ParseSpec("heatmap")
	case "hour": // Same as -p /hour.
		v.spec, err = axis.ParseSpec("/hour")
	case "month": // Same as -p year/month.
		v.spec, err = axis.ParseSpec("year/month")
	case "week":
		v.kind = viewWeek
	case "weekday":
		v.kind = viewWeekday
	case "days":
		v.kind = viewDays
	case "summary":
		v.kind = viewSummary
	case "tzshift":
		v.kind = viewTZShift
	default:
		return v, fmt.Errorf(
			"unknown view %q (want %s, or all)",
			name,
			strings.Join(termViews, ", "),
		)
	}
	return v, err
}

// parseViews reads a view list such as "heatmap,week".
// Views are drawn in the given order.
func parseViews(s string, format formatKind) ([]viewSel, error) {
	names := strings.Split(s, ",")
	if strings.EqualFold(strings.TrimSpace(s), "all") {
		names = termViews
		if format == formatSVG {
			names = svgViews
		}
	}
	views := make([]viewSel, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		v, err := parseView(name)
		if err != nil {
			return nil, err
		}
		if seen[v.name] {
			return nil, fmt.Errorf("view %q is given twice", v.name)
		}
		seen[v.name] = true
		if format == formatSVG && v.kind != viewGrid && v.kind != viewWeek {
			return nil, fmt.Errorf("view %q is not available in svg (svg: %s, or --pivot)",
				v.name, strings.Join(svgViews, ", "))
		}
		views = append(views, v)
	}
	return views, nil
}

// pivotName turns --pivot axes into a view name that works in file names.
// For example, "year/hour" becomes "year-hour".
func pivotName(p string) string {
	return strings.Trim(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(p)), "/", "-"), "-")
}

// resolveOut decides the output from -o and --svg-layout.
// An empty out means stdout.
// When split is true, out is a directory and each view gets its own file.
//
// Without --svg-layout, the output decides the layout.
// A directory means split. Anything else means one sheet file.
// If an explicit option conflicts with the output, resolveOut returns an error that names the conflict.
func resolveOut(o options, format formatKind) (out string, split bool, err error) {
	switch {
	case !o.outSet && format == formatSVG &&
		strings.EqualFold(strings.TrimSpace(o.svgLayout), "split"):
		// Split file names have a prefix (git-when.all.), so they go to the current directory.
		out = "."
	case !o.outSet:
		out = defaultOutNames[format] // term has no entry, so it stays empty (stdout).
	case o.out == "":
		return "", false, errors.New("--out: empty file name (use - for stdout)")
	case o.out != "-":
		out = o.out
	}
	// A trailing slash names a directory that may not exist yet.
	dir := out != "" && (strings.HasSuffix(out, "/") || isDir(out))

	if format != formatSVG {
		switch {
		case o.svgLayout != "":
			return "", false, errors.New("--svg-layout applies only to --format svg")
		case dir:
			return "", false, fmt.Errorf(
				"--out: %s is a directory (only --format svg writes files into a directory)", out)
		}
		return out, false, nil
	}

	switch strings.ToLower(strings.TrimSpace(o.svgLayout)) {
	case "":
		return out, dir, nil
	case "sheet":
		if dir {
			return "", false, fmt.Errorf(
				"--svg-layout sheet writes one file, but -o %s is a directory", out)
		}
		return out, false, nil
	case "split":
		switch {
		case out == "":
			return "", false, errors.New(
				"--svg-layout split writes one file per view and cannot write to stdout; use -o dir/",
			)
		case !dir:
			return "", false, fmt.Errorf("--svg-layout split writes one file per view, "+
				"but -o %s is not a directory (end it with / to create one)", out)
		}
		return out, true, nil
	}
	return "", false, fmt.Errorf("--svg-layout: unknown layout %q (want sheet, split)", o.svgLayout)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// output writes agg in the configured format.
// With --out it writes to that file. With SVG split it writes one file per view into the directory.
// All output is rendered before any file is created, so a render error leaves no partial files.
func output(stdout io.Writer, agg *model.Aggregate, cfg config) error {
	if cfg.out == "" {
		return write(stdout, agg, cfg, cfg.views)
	}

	var paths []string
	var bufs []*bytes.Buffer
	if cfg.split {
		for i, v := range cfg.views {
			var buf bytes.Buffer
			if err := write(&buf, agg, cfg, cfg.views[i:i+1]); err != nil {
				return err
			}
			paths = append(paths, filepath.Join(cfg.out, splitName(v)))
			bufs = append(bufs, &buf)
		}
		if err := os.MkdirAll(cfg.out, 0o750); err != nil {
			return err
		}
	} else {
		var buf bytes.Buffer
		if err := write(&buf, agg, cfg, cfg.views); err != nil {
			return err
		}
		paths, bufs = []string{cfg.out}, []*bytes.Buffer{&buf}
	}

	for i, path := range paths {
		//nolint:gosec // The user chose this path. Normal file permissions let other tools open it.
		if err := os.WriteFile(path, bufs[i].Bytes(), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// splitName returns the file name of one split SVG view.
// The pattern is {prefix}.{project}.{view}.svg.
// There is no project filter yet, so the project is always "all".
func splitName(v viewSel) string {
	return "git-when.all." + v.name + ".svg"
}

// write writes views to w in the configured format.
func write(w io.Writer, agg *model.Aggregate, cfg config, views []viewSel) error {
	switch cfg.format {
	case formatCSV:
		// CSV always has the metadata and header rows, even with no commits.
		// This keeps it easy to read from scripts.
		return render.CSV(w, agg)
	case formatSVG:
		return render.SVG(w, agg, svgSections(agg, cfg, views), cfg.render)
	case formatHTML:
		return render.HTML(w, agg)
	}

	total := agg.Total()
	if total == 0 {
		_, err := fmt.Fprintln(w, "No commits found.")
		return err
	}
	if _, err := fmt.Fprintf(w, "%d commits in %d repositories (author local time)\n",
		total, len(agg.Projects)); err != nil {
		return err
	}
	// Several views are stacked with a heading between them.
	for _, v := range views {
		heading := "\n"
		if len(views) > 1 {
			heading = "\n== " + sectionTitle(v) + " ==\n"
		}
		if _, err := io.WriteString(w, heading); err != nil {
			return err
		}
		if err := show(w, agg, cfg, v); err != nil {
			return err
		}
	}
	return nil
}

// svgSections turns views into SVG sections.
// resolve has already limited views to those that SVG can draw.
func svgSections(agg *model.Aggregate, cfg config, views []viewSel) []render.Section {
	out := make([]render.Section, len(views))
	for i, v := range views {
		out[i].Title = sectionTitle(v)
		if v.kind == viewWeek {
			wk := stats.SplitWeek(agg, cfg.weekend, nil)
			out[i].Week = &wk
			continue
		}
		g := axis.Fold(agg, v.spec, cfg.weekStart, nil)
		out[i].Grid = &g
	}
	return out
}

// show draws view v for the terminal.
func show(w io.Writer, agg *model.Aggregate, cfg config, v viewSel) error {
	switch v.kind {
	case viewWeek:
		return render.Week(w, stats.SplitWeek(agg, cfg.weekend, nil), cfg.render)
	case viewWeekday:
		days := stats.ByWeekday(agg, nil)
		return render.Weekdays(w, days, cfg.weekStart, cfg.weekend, cfg.render)
	case viewDays:
		days := stats.ByWeekday(agg, nil)
		return render.Days(w, days, cfg.weekStart, cfg.weekend, cfg.render)
	case viewSummary:
		rows, total := stats.Summarize(agg, cfg.weekend)
		return render.Summary(w, rows, total, cfg.render)
	case viewTZShift:
		return render.TZShift(w, stats.OffsetsByYear(agg, maxTZColumns), cfg.render)
	default:
		return render.Heatmap(w, axis.Fold(agg, v.spec, cfg.weekStart, nil), cfg.render)
	}
}

// sectionTitle returns the heading of a view.
func sectionTitle(v viewSel) string {
	switch v.kind {
	case viewWeek:
		return "Weekdays vs weekend by hour"
	case viewWeekday:
		return "Hours by weekday"
	case viewDays:
		return "Hours on each weekday"
	case viewSummary:
		return "Repositories"
	case viewTZShift:
		return "UTC offsets by year"
	}
	if v.spec.Y == axis.None {
		return "Commits by " + kindName(v.spec.X)
	}
	y := kindName(v.spec.Y)
	return strings.ToUpper(y[:1]) + y[1:] + " × " + kindName(v.spec.X)
}

func kindName(k axis.Kind) string {
	switch k {
	case axis.Hour:
		return "hour"
	case axis.Weekday:
		return "weekday"
	case axis.Month:
		return "month"
	case axis.Year:
		return "year"
	case axis.Project:
		return "repository"
	case axis.Author:
		return "author"
	}
	return "?"
}
