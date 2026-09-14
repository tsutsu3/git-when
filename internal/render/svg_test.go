package render

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/tsutsu3/git-when/internal/axis"
	"github.com/tsutsu3/git-when/internal/model"
	"github.com/tsutsu3/git-when/internal/stats"
)

func TestSVG(t *testing.T) {
	agg := svgSample(t)
	s := renderSVG(t, agg, Options{})

	root := svgRoot(t, s)
	attrs := map[string]string{}
	for _, a := range root.Attr {
		attrs[a.Name.Local] = a.Value
	}
	if root.Name.Local != "svg" || !strings.HasPrefix(attrs["viewBox"], "0 0 880 ") {
		t.Errorf("root = %s %v, want <svg viewBox=\"0 0 880 …\">", root.Name.Local, attrs)
	}
	// Only the viewBox sets the size, so the SVG follows the width of the page.
	if _, ok := attrs["width"]; ok {
		t.Error("root has a width attribute")
	}
	if _, ok := attrs["height"]; ok {
		t.Error("root has a height attribute")
	}

	for _, want := range []string{
		"<metadata>\ngit-when dev\ncommand: git-when -f svg --no-cache &apos;&lt;&amp;&gt;&apos;\n",
		"repository: head=abc123 commits=3 name=app\n</metadata>",
		"<title id=\"title\">git-when: Weekday × hour, Weekdays vs weekend</title>",
		"author local time · 2024-03 to 2024-03 · 3 commits · 1 repositories",
		// The largest cell is the darkest. Day and night use the same hue. Both have a <title>.
		`class="d4"><title>Tue 09: 2 commits</title>`,
		`class="d1"><title>Sun 23: 1 commit</title>`,
		`class="z"><title>Mon 00: 0 commits</title>`,
		// The legend is always present.
		">sqrt scale · max 2 at Tue 09 · empty cells = 0</text>",
		`class="d3"><title>weekdays 09:00: 2 commits</title>`,
		`class="d3"><title>weekend 23:00: 1 commit</title>`,
		// A weekend share of 1/3 divided by 2/7 is 1.17.
		">weekend index 1.17 (1.00 = as busy per day as weekdays)</text>",
		"@media (prefers-color-scheme:dark)",
		"HEAD abc123",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("svg lacks %q:\n%s", want, s)
		}
	}
	// Right-aligned numbers use the attribute. Some renderers ignore text-anchor in <style>.
	if !strings.Contains(s, `text-anchor="end" class="m">2</text>`) {
		t.Errorf("row totals are not right-aligned with a text-anchor attribute:\n%s", s)
	}
	if strings.Contains(s, "<!--") {
		t.Error("svg has an XML comment; metadata belongs in <metadata>")
	}
	if strings.Contains(s, `class="n`) || strings.Contains(s, ">night 22:00-05:59</text>") {
		t.Error("svg distinguishes daytime and nighttime colors")
	}
	if again := renderSVG(t, agg, Options{}); again != s {
		t.Error("the same input rendered different bytes")
	}
}

func TestSVGThemes(t *testing.T) {
	agg := svgSample(t)
	for _, c := range []struct {
		theme SVGTheme
		paper string
	}{
		{SVGLight, ".bg{fill:#faf8f4}"},
		{SVGDark, ".bg{fill:#1f2328}"},
	} {
		s := renderSVG(t, agg, Options{SVGTheme: c.theme})
		// An explicit theme fixes the colors, so viewer settings do not change them.
		if !strings.Contains(s, c.paper) || strings.Contains(s, "@media") {
			t.Errorf("theme %d: want %s without @media:\n%s", c.theme, c.paper, s)
		}
	}
}

func TestSVGHidesRows(t *testing.T) {
	s := renderSVG(t, svgSample(t), Options{MinRowTotal: 2})
	svgRoot(t, s)
	if !strings.Contains(s, ">6 of 7 weekday rows hidden: fewer than 2 commits</text>") {
		t.Errorf("svg lacks the hidden rows note:\n%s", s)
	}
	if strings.Contains(s, "<title>Sun 23: ") {
		t.Errorf("hidden Sun row is drawn:\n%s", s)
	}
}

func TestSVGWithoutCommits(t *testing.T) {
	s := renderSVG(t, model.New(), Options{})
	svgRoot(t, s)
	if strings.Count(s, ">no commits</text>") != 2 {
		t.Errorf("want a no commits note for each view:\n%s", s)
	}
}

func TestXtermHex(t *testing.T) {
	for code, want := range map[int]string{
		16: "#000000", 19: "#0000af", 130: "#af5f00", 223: "#ffd7af", 231: "#ffffff",
		232: "#080808", 244: "#808080", 255: "#eeeeee",
	} {
		if got := xtermHex(code); got != want {
			t.Errorf("xtermHex(%d) = %s, want %s", code, got, want)
		}
	}
}

func TestXMLText(t *testing.T) {
	got := xmlText("a<b>&\"'\x01\xff\tx")
	if want := "a&lt;b&gt;&amp;&quot;&apos;��\tx"; got != want {
		t.Errorf("xmlText = %q, want %q", got, want)
	}
}

// svgSample has two commits on Tue 09 and one on Sun 23 (night hours).
func svgSample(t *testing.T) *model.Aggregate {
	t.Helper()
	agg := model.New()
	agg.Projects = []model.Project{{Name: "app", Head: "abc123", Commits: 3}}
	agg.Authors = []model.Author{{Name: "Alice", Email: "alice@example.com"}}
	agg.Meta = model.Meta{
		Tool: "git-when", Version: "dev", TZ: "author",
		Command: "git-when -f svg --no-cache '<&>'",
	}
	for _, s := range []string{
		"2024-03-05T09:00:00+09:00",
		"2024-03-05T09:10:00+09:00",
		"2024-03-10T23:30:00-08:00",
	} {
		tm, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		_, offset := tm.Zone()
		if err := agg.Add(0, 0, tm, int32(offset)); err != nil { //nolint:gosec // A fixed offset.
			t.Fatal(err)
		}
	}
	return agg
}

func renderSVG(t *testing.T, agg *model.Aggregate, o Options) string {
	t.Helper()
	spec, err := axis.ParseSpec("heatmap")
	if err != nil {
		t.Fatal(err)
	}
	g := axis.Fold(agg, spec, 0, nil)
	wk := stats.SplitWeek(agg, stats.DefaultWeekend(), nil)
	var buf bytes.Buffer
	err = SVG(&buf, agg, []Section{
		{Title: "Weekday × hour", Grid: &g},
		{Title: "Weekdays vs weekend", Week: &wk},
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// svgRoot reads s as XML to the end and returns the root element.
// It fails the test when s is not well-formed.
func svgRoot(t *testing.T, s string) xml.StartElement {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(s))
	var root xml.StartElement
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return root
		}
		if err != nil {
			t.Fatalf("not well-formed XML: %v\n%s", err, s)
		}
		if se, ok := tok.(xml.StartElement); ok && root.Name.Local == "" {
			root = se
		}
	}
}
