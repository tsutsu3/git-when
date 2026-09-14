package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/tsutsu3/git-when/internal/collect"
	"github.com/tsutsu3/git-when/internal/model"
)

// shellSafe lists the characters that need no quoting in a shell.
const shellSafe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./=:,@+%"

// phaseTimes records how long each phase of one run took.
type phaseTimes struct {
	total    time.Duration
	discover time.Duration // Repository discovery.
	read     time.Duration // Reading and aggregating git log for all repositories.
	render   time.Duration // Rendering the views.
	dirs     int           // Directories visited.
	repos    int           // Repositories found.
	collect  collect.Timings
}

// meta returns the run conditions recorded in the output.
// They show later how the output was made.
// The HTML report also uses the week start and weekend for ordering and the weekend index.
func meta(o options, cfg config, args []string) model.Meta {
	var weekend []int
	for d, on := range cfg.weekend {
		if on {
			weekend = append(weekend, d)
		}
	}
	return model.Meta{
		Tool:         "git-when",
		Version:      version,
		TZ:           "author",
		WeekStart:    cfg.weekStart,
		Weekend:      weekend,
		MinTotal:     o.minTotal,
		NoGitHubLink: cfg.noGitHubLink,
		NoEmail:      cfg.noEmail,
		Command:      commandLine(args),
		Roots:        o.roots,
		Filters: model.Filters{
			Author:        o.filter.Author,
			ExcludeAuthor: o.filter.ExcludeAuthor,
			Bots:          o.filter.Bots,
			Rev:           "HEAD",
		},
	}
}

// defaultAuthorIndex returns the index of the author that the HTML report selects when it opens.
// expr matches "Name <email>", like --author.
// Among several matches, the author with the most commits wins. A tie goes to the earlier author.
func defaultAuthorIndex(agg *model.Aggregate, expr string) (int, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return 0, err
	}
	totals := make([]uint64, len(agg.Authors))
	for k, n := range agg.Buckets {
		if a := int(model.Unpack(k).Author); a < len(totals) {
			totals[a] += uint64(n)
		}
	}
	best := -1
	for i, a := range agg.Authors {
		if re.MatchString(a.Name+" <"+a.Email+">") && (best < 0 || totals[i] > totals[best]) {
			best = i
		}
	}
	if best < 0 {
		return 0, fmt.Errorf("no author matches %q", expr)
	}
	return best, nil
}

// commandLine joins args into one line that can be pasted back into a shell.
func commandLine(args []string) string {
	parts := []string{"git-when"}
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// shellQuote wraps s in single quotes if it has any character outside shellSafe.
// Single quotes inside s are escaped so the shell reads them back unchanged.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, shellSafe) == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// printTimings writes the phase timings.
// They show which part is slow: discovery, git, or parsing.
func printTimings(w io.Writer, t phaseTimes) {
	c := t.collect
	cacheInfo := "cache off"
	if c.CacheHits+c.CacheMisses > 0 {
		cacheInfo = fmt.Sprintf("cache %s (%d hits, %d misses)",
			fmtDuration(c.Cache), c.CacheHits, c.CacheMisses)
	}
	fmt.Fprintf(w, "\ntimings: total %s\n", fmtDuration(t.total))
	fmt.Fprintf(w, "  discover   %6s  %d dirs, %d repositories\n",
		fmtDuration(t.discover), t.dirs, t.repos)
	fmt.Fprintf(w, "  read logs  %6s  git %s, %s, parse %s, aggregate %s\n",
		fmtDuration(t.read), fmtDuration(c.Git), cacheInfo,
		fmtDuration(c.Parse), fmtDuration(c.Aggregate))
	fmt.Fprintf(w, "  render     %6s\n", fmtDuration(t.render))
}

// fmtDuration formats a duration with readable precision, such as "<1ms", "37ms", or "1.23s".
func fmtDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return "<1ms"
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}
