// Command git-when shows when commits happen across one or more Git repositories.
//
// It finds repositories under the given directories, reads their history, and
// draws the result as a terminal view, CSV, SVG, or a single-file HTML report.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/tsutsu3/git-when/internal/cache"
	"github.com/tsutsu3/git-when/internal/collect"
	"github.com/tsutsu3/git-when/internal/discover"
	"github.com/tsutsu3/git-when/internal/model"
	"github.com/tsutsu3/git-when/internal/progress"
	"github.com/tsutsu3/git-when/internal/render"
)

// Exit codes.
const (
	exitOK    = 0
	exitError = 1 // A runtime error, such as no repositories found.
	exitUsage = 2 // Invalid arguments.
)

// maxProjects is the number of repositories that fit in model.Key.Project (16 bits).
const maxProjects = 1 << 16

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// run executes git-when once and returns the exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args, stderr)
	if errors.Is(err, pflag.ErrHelp) {
		return exitOK
	}
	if err != nil {
		return fail(stderr, exitUsage, err)
	}
	if opts.showVersion {
		fmt.Fprintln(stdout, version)
		return exitOK
	}

	// Report argument errors before any directory walk or git command.
	cfg, err := resolve(opts, stdout, stderr)
	if err != nil {
		return fail(stderr, exitUsage, err)
	}
	filter, err := collect.NewFilter(opts.filter)
	if err != nil {
		return fail(stderr, exitUsage, err)
	}

	// A long walk or git log looks frozen without feedback.
	// Progress rewrites one stderr line and is cleared before warnings and after each phase.
	prog := progress.New(stderr, cfg.progress)

	start := time.Now()
	var times phaseTimes

	// One broken repository or directory must not stop the whole run.
	warn := func(path string, err error) {
		prog.Clear()
		fmt.Fprintf(stderr, "warning: skipping %s: %v\n", path, err)
	}

	repos, err := discover.Discover(opts.roots, discover.Options{
		MaxDepth:       opts.maxDepth,
		FollowSymlinks: opts.followSymlinks,
		Warn:           warn,
		Progress: func(dirs, found int, path string) {
			times.dirs = dirs
			prog.Update("discovering: %d dirs, %d repositories  %s",
				dirs, found, progress.TruncateLeft(path, 36))
		},
	})
	prog.Clear()
	times.discover, times.repos = time.Since(start), len(repos)
	if err != nil {
		return fail(stderr, exitError, err)
	}
	if len(repos) == 0 {
		return fail(stderr, exitError,
			fmt.Errorf("no git repositories found under %s", strings.Join(opts.roots, ", ")))
	}
	if len(repos) > maxProjects {
		return fail(stderr, exitError,
			fmt.Errorf("too many repositories: %d (max %d)", len(repos), maxProjects))
	}

	readStart := time.Now()
	col := &collect.Collector{Filter: filter, Cache: cache.New(cacheDirFor(opts))}
	agg := collectAll(
		ctx,
		repos,
		col,
		warn,
		func(i int, r discover.Repo) {
			prog.Set(
				"reading git log: %d/%d  %s",
				i+1,
				len(repos),
				progress.TruncateLeft(r.Name, 40),
			)
		},
	)
	prog.Clear()
	times.read, times.collect = time.Since(readStart), col.Timings
	agg.Meta = meta(opts, cfg, args)
	agg.Meta.Period = render.Period(agg)
	if cfg.defaultAuthor != "" {
		i, err := defaultAuthorIndex(agg, cfg.defaultAuthor)
		if err != nil {
			return fail(stderr, exitError, fmt.Errorf("--default-author: %w", err))
		}
		agg.Meta.DefaultAuthor = &i
	}

	renderStart := time.Now()
	if err := output(stdout, agg, cfg); err != nil {
		return fail(stderr, exitError, err)
	}
	// Tell the user where the files went, even for default file names.
	switch {
	case cfg.split:
		fmt.Fprintf(stderr, "wrote %d files to %s\n", len(cfg.views), cfg.out)
	case cfg.out != "":
		fmt.Fprintf(stderr, "wrote %s\n", cfg.out)
	}
	if cfg.timings {
		times.render, times.total = time.Since(renderStart), time.Since(start)
		printTimings(stderr, times)
	}
	return exitOK
}

func fail(stderr io.Writer, code int, err error) int {
	fmt.Fprintf(stderr, "error: %v\n", err)
	return code
}

// collectAll reads the commits of every repository into one aggregate.
// Repositories that cannot be read are passed to warn and skipped.
// If onRepo is not nil, it is called before each repository is read.
func collectAll(
	ctx context.Context, repos []discover.Repo, col *collect.Collector,
	warn func(string, error), onRepo func(i int, r discover.Repo),
) *model.Aggregate {
	agg := model.New()
	for i, r := range repos {
		if onRepo != nil {
			onRepo(i, r)
		}
		project := uint16(len(agg.Projects)) //nolint:gosec // run checks maxProjects.
		res, err := col.Collect(ctx, agg, project, r.Path)
		if err != nil {
			warn(r.Path, err)
			if res.Commits == 0 {
				// Nothing was added, so the next repository can reuse this index.
				continue
			}
		}
		agg.Projects = append(agg.Projects, model.Project{
			Name: r.Name, Path: r.Path, Head: res.Head, Commits: res.Commits,
		})
	}
	return agg
}
