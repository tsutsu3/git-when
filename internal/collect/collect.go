// Package collect reads commits from git log, filters them by author, and adds them to an aggregate.
package collect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tsutsu3/git-when/internal/cache"
	"github.com/tsutsu3/git-when/internal/model"
)

// DefaultBotPattern matches bot identities that are excluded by default.
// It is matched against Commit.Ident.
//
// It matches these identities.
//   - GitHub App names such as "dependabot[bot]", "github-actions[bot]", and "renovate[bot]".
//   - Names or emails with "bot" as a word, such as "Renovate Bot" or "bot@...".
//   - Names that start with a common dependency update tool.
//
// In repositories with many automated updates, bots would otherwise dominate the pattern.
const DefaultBotPattern = `(?i)\[bot\]|\bbot\b|^(dependabot|renovate)\b`

// logFormat prints one line per commit with the author date, name, and email separated by tabs.
// The upper-case %aN and %aE apply .mailmap together with --use-mailmap.
const logFormat = "--pretty=format:%aI%x09%aN%x09%aE"

// cacheVersion is the format version of cached git log output.
// Change it when logFormat or the git log arguments change.
// It is part of the key, so old entries are no longer used.
const cacheVersion = "log/1"

var botRE = regexp.MustCompile(DefaultBotPattern)

// Commit is one commit read from git log.
type Commit struct {
	// Time is the author date. It keeps the UTC offset that the author recorded.
	Time time.Time
	// Name is the author name after .mailmap.
	Name string
	// Email is the author email after .mailmap.
	Email string
}

// Ident returns the "Name <email>" string that filters match against.
// A name alone can collide with another person. An email alone cannot match a display name.
func (c Commit) Ident() string {
	return c.Name + " <" + c.Email + ">"
}

// FilterOptions holds the author filters.
// Each regular expression is matched anywhere in "Name <email>".
type FilterOptions struct {
	// Author keeps only matching authors. Empty keeps everyone.
	Author string
	// ExcludeAuthor drops matching authors. Empty drops no one.
	ExcludeAuthor string
	// Bots keeps bot commits when true.
	Bots bool
}

// Filter filters commits by author. Create it with NewFilter.
type Filter struct {
	author  *regexp.Regexp
	exclude *regexp.Regexp
	bots    bool
}

// NewFilter compiles the regular expressions in o.
// It returns an error for an invalid regular expression.
func NewFilter(o FilterOptions) (*Filter, error) {
	author, err := compile("author", o.Author)
	if err != nil {
		return nil, err
	}
	exclude, err := compile("exclude-author", o.ExcludeAuthor)
	if err != nil {
		return nil, err
	}
	return &Filter{author: author, exclude: exclude, bots: o.Bots}, nil
}

// Match reports whether c stays in the aggregate. A nil Filter always returns true.
// Bots are excluded before --author is applied. Set Bots to count them.
func (f *Filter) Match(c Commit) bool {
	if f == nil {
		return true
	}
	id := c.Ident()
	switch {
	case !f.bots && IsBot(id):
		return false
	case f.author != nil && !f.author.MatchString(id):
		return false
	case f.exclude != nil && f.exclude.MatchString(id):
		return false
	}
	return true
}

// Timings holds the total time a Collector spent in each phase.
type Timings struct {
	// Git is the time spent running git rev-parse and git log.
	Git time.Duration
	// Cache is the time spent reading and writing the cache.
	Cache time.Duration
	// Parse is the time spent parsing git log output.
	Parse time.Duration
	// Aggregate is the time spent filtering and adding commits.
	Aggregate time.Duration
	// CacheHits counts repositories read from the cache.
	CacheHits int
	// CacheMisses counts repositories that were not cached and ran git log.
	CacheMisses int
}

// Collector reads commits from repositories, filters them, and adds them to an aggregate.
// The zero value neither filters nor caches.
type Collector struct {
	// Filter selects commits. A nil Filter keeps every commit.
	Filter *Filter
	// Cache stores git log output. A nil Cache disables caching.
	Cache *cache.Cache
	// Timings grows with each call to Collect.
	Timings Timings
}

// Collect adds the commits of dir that pass Filter to agg under the given project index.
// It returns the number of commits added and HEAD.
// If an error happens midway, the commits added before it stay in agg.
func (c *Collector) Collect(
	ctx context.Context, agg *model.Aggregate, project uint16, dir string,
) (Result, error) {
	commits, head, err := c.log(ctx, dir)
	if err != nil {
		return Result{}, err
	}

	start := time.Now()
	defer func() { c.Timings.Aggregate += time.Since(start) }()
	res := Result{Head: head}
	for _, commit := range commits {
		if !c.Filter.Match(commit) {
			continue
		}
		author, err := agg.AuthorIndex(model.Author{Name: commit.Name, Email: commit.Email})
		if err != nil {
			return res, err
		}
		_, offset := commit.Time.Zone()
		//nolint:gosec // UTC offsets are within ±26 hours, so they fit in int32.
		if err := agg.Add(author, project, commit.Time, int32(offset)); err != nil {
			return res, fmt.Errorf("%s: %w", dir, err)
		}
		res.Commits++
	}
	return res, nil
}

// log works like Log and also returns HEAD at read time.
// If Cache is not nil, it caches the git log output.
//
// The cache holds the output before filtering, so it still works when --author changes.
// A cache read or write error only means that git log runs again. The result is the same.
func (c *Collector) log(
	ctx context.Context, dir string,
) (commits []Commit, head string, err error) {
	start := time.Now()
	head = revParseHead(ctx, dir)
	c.Timings.Git += time.Since(start)

	// Repositories without a readable HEAD (for example with no commits) are not cached.
	key, cacheable := "", c.Cache != nil && head != ""
	if cacheable {
		key = cacheKey(dir, head)
		start := time.Now()
		out, ok := c.Cache.Get(key)
		c.Timings.Cache += time.Since(start)
		if ok {
			c.Timings.CacheHits++
			commits, err = c.parse(out)
			return commits, head, err
		}
		c.Timings.CacheMisses++
	}

	start = time.Now()
	out, err := gitLog(ctx, dir)
	c.Timings.Git += time.Since(start)
	if err != nil {
		return nil, "", err
	}
	if cacheable {
		start := time.Now()
		_ = c.Cache.Put(key, out) // If this fails, the next run calls git log again.
		c.Timings.Cache += time.Since(start)
	}
	commits, err = c.parse(out)
	return commits, head, err
}

func (c *Collector) parse(out []byte) ([]Commit, error) {
	start := time.Now()
	defer func() { c.Timings.Parse += time.Since(start) }()
	return ParseLog(out)
}

// Result is what Collect read from one repository.
type Result struct {
	// Commits is the number of commits added after filtering.
	Commits int
	// Head is the HEAD commit hash at read time. It is empty when there are no commits.
	Head string
}

// Log returns the non-merge commits reachable from HEAD in the repository at dir.
// A repository without commits (right after git init) returns no commits and no error.
func Log(ctx context.Context, dir string) ([]Commit, error) {
	commits, _, err := (&Collector{}).log(ctx, dir)
	return commits, err
}

// ParseLog reads the output that Log asks git to print.
// A line in an unexpected format is an error. It is never dropped silently.
func ParseLog(out []byte) ([]Commit, error) {
	var commits []Commit
	for i, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		// The date ends at the first tab and the email starts after the last tab.
		// This keeps names with tabs intact.
		date, rest, ok := strings.Cut(line, "\t")
		sep := strings.LastIndexByte(rest, '\t')
		if !ok || sep < 0 {
			return nil, fmt.Errorf("git log line %d: unexpected format %q", i+1, line)
		}
		t, err := time.Parse(time.RFC3339, date)
		if err != nil {
			return nil, fmt.Errorf("git log line %d: %w", i+1, err)
		}
		commits = append(commits, Commit{Time: t, Name: rest[:sep], Email: rest[sep+1:]})
	}
	return commits, nil
}

// IsBot reports whether ident ("Name <email>") looks like a bot.
func IsBot(ident string) bool {
	return botRE.MatchString(ident)
}

// compile returns nil when expr is empty.
func compile(name, expr string) (*regexp.Regexp, error) {
	if expr == "" {
		return nil, nil
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid regular expression: %w", name, err)
	}
	return re, nil
}

// revParseHead returns the HEAD commit hash of dir.
// It returns an empty string when there are no commits or HEAD cannot be read.
func revParseHead(ctx context.Context, dir string) string {
	//nolint:gosec // dir is a repository path from the user. No shell is involved.
	out, err := exec.CommandContext(ctx, "git", "-C", dir,
		"rev-parse", "--verify", "-q", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// cacheKey builds the cache key from everything that decides the git log output of dir.
// That is the path, HEAD, the log format, and the .mailmap in the work tree.
//
// Git settings such as mailmap.file are not part of the key. Use --no-cache after changing them.
func cacheKey(dir, head string) string {
	// .mailmap changes %aN and %aE even when it is not committed, so its content is part of the key.
	mailmap, _ := os.ReadFile(filepath.Join(dir, ".mailmap")) //nolint:gosec // Fixed name.
	return cache.Key(cacheVersion, dir, head, logFormat, string(mailmap))
}

// gitLog runs git log and returns its output.
// It returns nothing for a repository without commits.
func gitLog(ctx context.Context, dir string) ([]byte, error) {
	// Options go before revisions. Bare repositories reject options after them.
	//nolint:gosec // dir is a repository path from the user. No shell is involved.
	cmd := exec.CommandContext(ctx, "git", "-C", dir,
		"log", "--no-merges", "--use-mailmap", logFormat)
	out, err := cmd.Output()
	if err != nil {
		// git log fails in an empty repository. Check the cause only after a failure.
		if hasNoCommits(ctx, dir) {
			return nil, nil
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("git log in %s: %w: %s",
				dir, err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("git log in %s: %w", dir, err)
	}
	return out, nil
}

// hasNoCommits reports whether HEAD in dir points to a branch that has no commits yet.
// It returns false when dir is not a repository or is broken.
func hasNoCommits(ctx context.Context, dir string) bool {
	// First make sure HEAD reads as a branch reference, which means the repository opens.
	//nolint:gosec // dir is a repository path from the user. No shell is involved.
	if exec.CommandContext(ctx, "git", "-C", dir, "symbolic-ref", "-q", "HEAD").Run() != nil {
		return false
	}
	// When the branch points to no commit, rev-parse fails with exit code 1.
	//nolint:gosec // Same as above.
	err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--verify", "-q", "HEAD").Run()
	var ee *exec.ExitError
	return errors.As(err, &ee) && ee.ExitCode() == 1
}
