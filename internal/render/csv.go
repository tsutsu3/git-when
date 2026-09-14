package render

import (
	"bufio"
	"cmp"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/tsutsu3/git-when/internal/model"
)

// csvHeader lists the CSV columns.
// Each row is one bucket (repository, author, year, month, weekday, and hour).
var csvHeader = []string{
	"project", "author_name", "author_email", "year", "month", "weekday", "hour", "commits",
}

// CSV writes agg as tidy (long) CSV.
// It is meant for spreadsheet pivots, pandas, and R.
//
// The format follows these rules.
//   - month is 1 for January to 12 for December.
//   - weekday is ISO 8601, from 1 for Monday to 7 for Sunday. Zero-based numbers confuse spreadsheets.
//   - hour is 0-23 in author local time.
//   - Buckets with no commits are not written.
//   - The leading # lines record the run conditions, such as the command line, period,
//     filters, and the HEAD of each repository.
//     pandas skips them with read_csv(path, comment="#").
//     R skips them with read.csv(path, comment.char="#").
//
// Rows are sorted by project, author, year, month, weekday, and hour.
// Projects and authors use their index in the aggregate.
func CSV(w io.Writer, agg *model.Aggregate) error {
	bw := bufio.NewWriter(w)
	writeCSVMeta(bw, agg)
	fmt.Fprintln(bw, strings.Join(csvHeader, ","))

	keys := slices.Collect(maps.Keys(agg.Buckets))
	slices.SortFunc(keys, func(a, b uint64) int {
		return cmp.Compare(projectFirst(a), projectFirst(b))
	})
	for _, u := range keys {
		k := model.Unpack(u)
		var author model.Author
		if int(k.Author) < len(agg.Authors) {
			author = agg.Authors[k.Author]
		}
		project := ""
		if int(k.Project) < len(agg.Projects) {
			project = agg.Projects[k.Project].Name
		}
		fmt.Fprintf(bw, "%s,%s,%s,%d,%d,%d,%d,%d\n",
			csvQuote(project), csvQuote(author.Name), csvQuote(author.Email),
			k.Year, k.Month+1, k.Weekday+1, k.Hour, agg.Buckets[u])
	}
	return bw.Flush() // bufio.Writer keeps earlier write errors and returns them here.
}

// Period returns the first and last month in the aggregate, such as "2023-12 to 2024-03".
// It returns "-" when there are no buckets.
func Period(agg *model.Aggregate) string {
	lo, hi := -1, -1
	for u := range agg.Buckets {
		k := model.Unpack(u)
		ym := int(k.Year)*12 + int(k.Month)
		if lo < 0 || ym < lo {
			lo = ym
		}
		hi = max(hi, ym)
	}
	if lo < 0 {
		return "-"
	}
	return fmt.Sprintf("%04d-%02d to %04d-%02d", lo/12, lo%12+1, hi/12, hi%12+1)
}

// projectFirst swaps project and author in a packed key.
// Author is the highest field of a packed key, so plain sorting would group rows by author.
func projectFirst(u uint64) uint64 {
	k := model.Unpack(u)
	k.Author, k.Project = k.Project, k.Author
	return k.Pack()
}

// csvQuote always wraps a string column in double quotes and doubles any quote inside.
// Without quotes, a name that starts with # would be dropped as a comment by readers
// that use comment="#".
func csvQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// writeCSVMeta writes the run conditions as # lines at the top of the CSV.
// Months later, they still show how the CSV was made.
func writeCSVMeta(w io.Writer, agg *model.Aggregate) {
	for _, l := range metaLines(agg) {
		fmt.Fprintf(w, "# %s\n", l)
	}
	fmt.Fprintln(w, "# month: 1=Jan .. 12=Dec; weekday: 1=Mon .. 7=Sun (ISO 8601); "+
		"hour: 0-23 in author local time")
	for _, l := range repoLines(agg) {
		fmt.Fprintf(w, "# %s\n", l)
	}
}

// metaLines returns the run conditions with one item per line.
// The items are the tool version, command line, time basis, period, and filters.
// CSV # lines and SVG <metadata> both use them.
func metaLines(agg *model.Aggregate) []string {
	m := agg.Meta
	return []string{
		metaText(toolName(m)),
		"command: " + metaText(m.Command),
		"time zone: " + metaText(m.TZ),
		"period: " + Period(agg),
		"filters: " + filterText(m.Filters),
	}
}

// repoLines returns one line per repository with its HEAD and commit count.
func repoLines(agg *model.Aggregate) []string {
	out := make([]string, len(agg.Projects))
	for i, p := range agg.Projects {
		head := p.Head
		if head == "" {
			head = "-" // A repository without commits.
		}
		out[i] = fmt.Sprintf("repository: head=%s commits=%d name=%s",
			metaText(head), p.Commits, metaText(p.Name))
	}
	return out
}

// toolName returns the tool name and version, such as "git-when dev".
func toolName(m model.Meta) string {
	return strings.TrimSpace(m.Tool + " " + m.Version)
}

// metaText returns a value for a # line.
// A control character such as a newline would split the line and look like a CSV row.
// In that case the value is written as a quoted Go string.
func metaText(s string) string {
	if strings.ContainsFunc(s, unicode.IsControl) {
		return strconv.Quote(s)
	}
	return s
}

// filterText writes the filters in one line.
// Regular expressions are quoted so their boundaries are clear.
func filterText(f model.Filters) string {
	var parts []string
	if f.Author != "" {
		parts = append(parts, "author="+strconv.Quote(f.Author))
	}
	if f.ExcludeAuthor != "" {
		parts = append(parts, "exclude-author="+strconv.Quote(f.ExcludeAuthor))
	}
	if f.Bots {
		parts = append(parts, "bots included")
	} else {
		parts = append(parts, "bots excluded")
	}
	if f.Merges {
		parts = append(parts, "merges included")
	} else {
		parts = append(parts, "merges excluded")
	}
	return strings.Join(parts, ", ")
}
