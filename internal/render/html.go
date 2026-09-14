package render

import (
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/tsutsu3/git-when/internal/model"
)

// The template has exactly one data block between these markers.
// The block holds one script that sets window.__GIT_WHEN_DATA__.
// HTML replaces the whole block, markers included, with a script that holds the real data.
const (
	htmlDataStart = "<!--@git-when-data-start-->"
	htmlDataEnd   = "<!--@git-when-data-end-->"
)

// htmlTemplate is the single-file HTML report built from frontend/.
// Opened directly in a browser, it shows sample data.
//
//go:generate pnpm --dir ../.. build:template
//go:embed template.html
var htmlTemplate string

// userHomeDir finds the home directory that publicAggregate hides. Tests replace it.
var userHomeDir = os.UserHomeDir

// HTML writes a self-contained HTML report.
// It needs no CDN or network and opens from file://.
// The aggregate is embedded as JSON, and the browser counts it again when filters change.
// Local paths are left out, because the report is often shared or published.
func HTML(w io.Writer, agg *model.Aggregate) error {
	out, err := fillTemplate(htmlTemplate, publicAggregate(agg))
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, out)
	return err
}

// fillTemplate replaces the sample data block in tmpl with the JSON of agg.
func fillTemplate(tmpl string, agg *model.Aggregate) (string, error) {
	// json.Marshal escapes <, >, &, U+2028, and U+2029 in strings, for example as \u003c.
	// A repository or author name with "</script>" or "<!--" cannot end the script early
	// or change how the browser parses the page.
	data, err := json.Marshal(agg)
	if err != nil {
		return "", err
	}

	if strings.Count(tmpl, htmlDataStart) != 1 || strings.Count(tmpl, htmlDataEnd) != 1 {
		return "", errors.New("html template: the data markers are missing")
	}
	start := strings.Index(tmpl, htmlDataStart)
	end := strings.Index(tmpl, htmlDataEnd)
	if end < start {
		return "", errors.New("html template: the data markers are out of order")
	}
	end += len(htmlDataEnd)
	inject := "<script>window.__GIT_WHEN_DATA__=" + string(data) + ";</script>"
	return tmpl[:start] + inject + tmpl[end:], nil
}

// publicAggregate returns a copy of agg without local paths. agg itself is not changed.
//
// The page never shows these paths.
//   - Project paths are empty.
//   - The search roots are an empty list.
//   - The home directory in the command line is written as "~".
//   - With Meta.NoEmail, author emails are empty. Authors keep their indexes.
func publicAggregate(agg *model.Aggregate) *model.Aggregate {
	public := *agg
	public.Projects = make([]model.Project, len(agg.Projects))
	for i, p := range agg.Projects {
		p.Path = ""
		public.Projects[i] = p
	}
	public.Meta.Roots = []string{}
	if home, err := userHomeDir(); err == nil {
		public.Meta.Command = tildeHome(agg.Meta.Command, home)
	}
	if agg.Meta.NoEmail {
		// Authors are not merged. The page numbers authors that share a name.
		public.Authors = make([]model.Author, len(agg.Authors))
		for i, a := range agg.Authors {
			public.Authors[i] = model.Author{Name: a.Name}
		}
	}
	return &public
}

// tildeHome replaces home with "~" where home is a whole path prefix in the command line s.
// For example, "/home/tt/src" becomes "~/src", but "/home/tt2" and "/x/home/tt" do not change.
func tildeHome(s, home string) string {
	home = strings.TrimRight(home, `/\`)
	if home == "" {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, home)
		if i < 0 {
			break
		}
		end := i + len(home)
		starts := i == 0 || strings.IndexByte(` '=`, s[i-1]) >= 0
		ends := end == len(s) || strings.IndexByte(`/\ '`, s[end]) >= 0
		b.WriteString(s[:i])
		if starts && ends {
			b.WriteString("~")
		} else {
			b.WriteString(home)
		}
		s = s[end:]
	}
	b.WriteString(s)
	return b.String()
}
