package render

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tsutsu3/git-when/internal/model"
)

const dataPrefix = "<script>window.__GIT_WHEN_DATA__="

// lineSeparator is U+2028. JSON allows it in strings, but older JavaScript treats it as a line break.
var lineSeparator = string(rune(0x2028))

func TestHTML(t *testing.T) {
	agg := htmlSample(t)
	var buf bytes.Buffer
	if err := HTML(&buf, agg); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, bad := range []string{
		"</script><script>alert", // </script> in a name would end the script.
		"<!--",                   // <!-- inside a script changes how browsers parse it.
		lineSeparator,
		"generateSample", // No trace of the sample generator remains.
		htmlDataStart,
		htmlDataEnd,
	} {
		if strings.Contains(out, bad) {
			t.Errorf("html contains %q", bad)
		}
	}
	// There are only two </script> tags, one for the data and one for the application.
	if n := strings.Count(out, "</script>"); n != 2 {
		t.Errorf("html has %d </script>, want 2", n)
	}
	if n := strings.Count(out, "window.__GIT_WHEN_DATA__"); n != 2 {
		t.Errorf("html references the embedded data %d times, want data and app scripts", n)
	}
	start := strings.Index(out, dataPrefix)
	if start < 0 {
		t.Fatal("the data is not injected")
	}

	// The injected data parses as JSON and the names come back unchanged.
	rest := out[start+len(dataPrefix):]
	var doc struct {
		Projects []struct{ Name string } `json:"projects"`
		Authors  []struct{ Name string } `json:"authors"`
		Buckets  []int                   `json:"buckets"`
		Meta     struct {
			Command  string `json:"command"`
			Weekend  []int  `json:"weekend"`
			MinTotal int    `json:"minTotal"`
		} `json:"meta"`
	}
	end := strings.Index(rest, ";</script>")
	if end < 0 {
		t.Fatal("the data script is not closed")
	}
	if err := json.Unmarshal([]byte(rest[:end]), &doc); err != nil {
		t.Fatalf("embedded data is not JSON: %v", err)
	}
	if len(doc.Projects) != 1 || doc.Projects[0].Name != agg.Projects[0].Name ||
		len(doc.Authors) != 1 || doc.Authors[0].Name != agg.Authors[0].Name ||
		doc.Meta.Command != agg.Meta.Command || len(doc.Meta.Weekend) != 2 ||
		doc.Meta.MinTotal != agg.Meta.MinTotal {
		t.Errorf("embedded data = %+v", doc)
	}
	if len(doc.Buckets) != model.BucketStride {
		t.Errorf("buckets = %v, want one bucket of %d numbers", doc.Buckets, model.BucketStride)
	}
}

// TestHTMLTemplateMarkers checks that missing data markers give an error
// instead of broken HTML.
func TestHTMLTemplateMarkers(t *testing.T) {
	// The template has a sample script and an application script. Only the sample is replaced.
	if n := strings.Count(htmlTemplate, "</script>"); n != 2 {
		t.Errorf("template has %d </script>, want sample data and app scripts", n)
	}

	agg := htmlSample(t)
	for name, c := range map[string]struct{ old, new string }{
		"data start": {htmlDataStart, "<!--data-start-->"},
		"data end":   {htmlDataEnd, "<!--data-end-->"},
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(htmlTemplate, c.old) {
				t.Fatalf("template has no %q", c.old)
			}
			broken := strings.ReplaceAll(htmlTemplate, c.old, c.new)
			if _, err := fillTemplate(broken, agg); err == nil {
				t.Error("fillTemplate succeeded without the marker")
			}
		})
	}
}

func TestHTMLHeatmapHasNoWeekdayWeekendGap(t *testing.T) {
	if strings.Contains(htmlTemplate, "gap-top") {
		t.Error("heatmap separates weekdays and weekends with a gap")
	}
}

// TestHTMLHidesLocalPaths checks that a published report does not show where the repositories are.
func TestHTMLHidesLocalPaths(t *testing.T) {
	userHomeDir = func() (string, error) { return "/home/alice", nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	agg := htmlSample(t)
	agg.Projects[0].Path = "/home/alice/src/app"
	agg.Meta.Roots = []string{"/home/alice/src"}
	agg.Meta.Command = "git-when /home/alice/src -o /home/alice/public/index.html /home/alice2"
	var buf bytes.Buffer
	if err := HTML(&buf, agg); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`"path":""`,
		`"roots":[]`,
		// The home directory of a similar name is another directory, so it stays.
		`"command":"git-when ~/src -o ~/public/index.html /home/alice2"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("html lacks %s", want)
		}
	}
	if strings.Contains(out, "/home/alice/") {
		t.Error("html still contains the home directory")
	}
	// The caller's aggregate is not changed.
	if agg.Projects[0].Path != "/home/alice/src/app" || len(agg.Meta.Roots) != 1 {
		t.Errorf("HTML changed the aggregate: %+v", agg)
	}

	for _, c := range []struct{ in, want string }{
		{"git-when /home/alice", "git-when ~"},
		{"git-when '/home/alice/my src'", "git-when '~/my src'"},
		{"git-when --cache-dir=/home/alice/c .", "git-when --cache-dir=~/c ."},
		{"git-when /x/home/alice/src", "git-when /x/home/alice/src"},
	} {
		if got := tildeHome(c.in, "/home/alice/"); got != c.want {
			t.Errorf("tildeHome(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHTMLNoEmail checks that NoEmail removes emails and keeps authors that share a name apart.
func TestHTMLNoEmail(t *testing.T) {
	agg := model.New()
	agg.Projects = []model.Project{{Name: "app", Commits: 4}}
	agg.Authors = []model.Author{
		{Name: "tsutsu3", Email: "old@example.com"},
		{Name: "alice", Email: "alice@example.com"},
		{Name: "tsutsu3", Email: "new@example.com"},
	}
	tm := time.Date(2024, 3, 5, 9, 0, 0, 0, time.FixedZone("", 9*3600)) // Tuesday 09:00
	for _, author := range []uint16{0, 1, 2, 2} {
		if err := agg.Add(author, 0, tm, 9*3600); err != nil {
			t.Fatal(err)
		}
	}
	selected := 2
	agg.Meta = model.Meta{Weekend: []int{5, 6}, NoEmail: true, DefaultAuthor: &selected}

	var buf bytes.Buffer
	if err := HTML(&buf, agg); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "@example.com") {
		t.Error("html still contains an email")
	}
	start := strings.Index(out, dataPrefix)
	if start < 0 {
		t.Fatal("the data is not injected")
	}
	rest := out[start+len(dataPrefix):]
	end := strings.Index(rest, ";</script>")
	if end < 0 {
		t.Fatal("the data script is not closed")
	}
	var doc struct {
		Authors []struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"authors"`
		Buckets []int `json:"buckets"`
		Offsets []int `json:"offsets"`
		Meta    struct {
			DefaultAuthor int `json:"defaultAuthor"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(rest[:end]), &doc); err != nil {
		t.Fatal(err)
	}

	// The authors keep their order, so the two tsutsu3 identities stay apart. Only the emails are gone.
	names := make([]string, len(doc.Authors))
	for i, a := range doc.Authors {
		names[i] = a.Name
		if a.Email != "" {
			t.Errorf("author %d has email %q", i, a.Email)
		}
	}
	if want := []string{"tsutsu3", "alice", "tsutsu3"}; !slices.Equal(names, want) {
		t.Errorf("authors = %q, want %q", names, want)
	}
	// author, project, yearIndex, month, weekday, hour, count
	want := []int{0, 0, 0, 2, 1, 9, 1, 1, 0, 0, 2, 1, 9, 1, 2, 0, 0, 2, 1, 9, 2}
	if !slices.Equal(doc.Buckets, want) {
		t.Errorf("buckets = %v, want %v", doc.Buckets, want)
	}
	// author, project, yearIndex, offsetSeconds, count
	if want := []int{0, 0, 0, 32400, 1, 1, 0, 0, 32400, 1, 2, 0, 0, 32400, 2}; !slices.Equal(
		doc.Offsets, want) {
		t.Errorf("offsets = %v, want %v", doc.Offsets, want)
	}
	if doc.Meta.DefaultAuthor != 2 {
		t.Errorf("defaultAuthor = %d, want 2", doc.Meta.DefaultAuthor)
	}
	// The caller's aggregate is not changed.
	if agg.Authors[2].Email != "new@example.com" {
		t.Errorf("HTML changed the aggregate: %+v", agg.Authors)
	}
}

// htmlSample has names with characters that could break HTML or scripts.
func htmlSample(t *testing.T) *model.Aggregate {
	t.Helper()
	agg := model.New()
	agg.Projects = []model.Project{
		{Name: "</script><script>alert(1)</script>", Head: "abc123", Commits: 1},
	}
	agg.Authors = []model.Author{
		{Name: "Line" + lineSeparator + "Separator", Email: "a&b@example.com"},
	}
	agg.Meta = model.Meta{
		Tool: "git-when", Version: "dev", TZ: "author", Weekend: []int{5, 6}, MinTotal: 2,
		Command: "git-when -f html <!-- --no-cache",
	}
	tm := time.Date(2024, 3, 5, 9, 0, 0, 0, time.FixedZone("", 9*3600))
	if err := agg.Add(0, 0, tm, 9*3600); err != nil {
		t.Fatal(err)
	}
	return agg
}
