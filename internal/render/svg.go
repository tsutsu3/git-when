package render

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/tsutsu3/git-when/internal/axis"
	"github.com/tsutsu3/git-when/internal/model"
	"github.com/tsutsu3/git-when/internal/stats"
)

// SVG sizes in pixels.
// All coordinates are integers, so the output is deterministic and diffs are easy to read.
const (
	svgWidth   = 880 // viewBox width. The height comes from the content.
	svgMargin  = 24
	charPx     = 7  // Rough width of one half-width character in a 12px system font.
	lineStep   = 18 // Line height of body text.
	maxCellPx  = 32
	minRowPx   = 14
	maxRowPx   = 24
	maxLabelPx = 200
	swatchPx   = 12
	weekRowPx  = 16
	weekHourPx = 36 // Width of the center hour column in the week view.
)

// svgFont is the text style shared by all color schemes.
// It uses only system fonts and tabular numbers.
const svgFont = `text{font-family:system-ui,-apple-system,"Segoe UI",Roboto,"Noto Sans",sans-serif;` +
	`font-size:12px;font-variant-numeric:tabular-nums}
.h1{font-size:18px;font-weight:600}.h2{font-size:14px;font-weight:600}
`

var xmlEscaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
)

// SVGTheme is the color scheme of SVG output.
type SVGTheme int

// SVG color schemes. The default is SVGAuto.
const (
	// SVGAuto writes light colors and adds dark colors for prefers-color-scheme: dark.
	// Tools such as Inkscape ignore @media and show the light colors.
	SVGAuto SVGTheme = iota
	// SVGLight always uses light colors.
	SVGLight
	// SVGDark always uses dark colors.
	SVGDark
)

// ParseSVGTheme reads "auto", "light", or "dark".
func ParseSVGTheme(s string) (SVGTheme, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "auto":
		return SVGAuto, nil
	case "light":
		return SVGLight, nil
	case "dark":
		return SVGDark, nil
	}
	return SVGAuto, fmt.Errorf("unknown theme %q (want auto, light, dark)", s)
}

// Section is one view in an SVG. Set exactly one of Grid and Week.
type Section struct {
	// Title is the heading of the view.
	Title string
	// Grid is drawn as a heatmap.
	Grid *axis.Grid
	// Week is drawn as weekday and weekend bars.
	Week *stats.Week
}

// svgDoc builds the SVG body from top to bottom.
// y is the top of the next element.
type svgDoc struct {
	b strings.Builder
	y int
}

// text writes s with its baseline at (x, y).
// class is space-separated. "e" aligns to the end and "c" aligns to the center.
//
// Alignment uses the text-anchor attribute instead of CSS.
// Tools such as ImageMagick ignore text-anchor in <style>, and numbers would overlap bars and labels.
func (d *svgDoc) text(x, y int, class, s string) {
	var attrs strings.Builder
	var classes []string
	for _, c := range strings.Fields(class) {
		switch c {
		case "e":
			attrs.WriteString(` text-anchor="end"`)
		case "c":
			attrs.WriteString(` text-anchor="middle"`)
		default:
			classes = append(classes, c)
		}
	}
	if len(classes) > 0 {
		fmt.Fprintf(&attrs, ` class="%s"`, strings.Join(classes, " "))
	}
	fmt.Fprintf(&d.b, "<text x=\"%d\" y=\"%d\"%s>%s</text>\n", x, y, attrs.String(), xmlText(s))
}

// rect draws a rectangle.
// If title is not empty, it adds a <title> that browsers show as a tooltip.
func (d *svgDoc) rect(x, y, w, h int, class, title string) {
	fmt.Fprintf(&d.b, "<rect x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\" class=\"%s\"",
		x, y, w, h, class)
	if title == "" {
		d.b.WriteString("/>\n")
		return
	}
	fmt.Fprintf(&d.b, "><title>%s</title></rect>\n", xmlText(title))
}

func (d *svgDoc) header(agg *model.Aggregate) {
	d.y += 18
	d.text(svgMargin, d.y, "h1", "git-when")
	d.y += lineStep
	d.text(svgMargin, d.y, "m", summaryLine(agg))
	d.y += 4
}

// rule draws one line between views. Views have no frames or background blocks.
func (d *svgDoc) rule() {
	d.y += 10
	fmt.Fprintf(&d.b, "<line x1=\"%d\" y1=\"%d\" x2=\"%d\" y2=\"%d\" class=\"rule\"/>\n",
		svgMargin, d.y, svgWidth-svgMargin, d.y)
}

func (d *svgDoc) heading(s string) {
	d.y += 24
	d.text(svgMargin, d.y, "h2", s)
	d.y += 4
}

func (d *svgDoc) note(s string) {
	d.y += lineStep
	d.text(svgMargin, d.y, "m", s)
}

func (d *svgDoc) footer(agg *model.Aggregate) {
	s := fmt.Sprintf("%s · %d repositories", toolName(agg.Meta), len(agg.Projects))
	if len(agg.Projects) == 1 && agg.Projects[0].Head != "" {
		s += " · HEAD " + agg.Projects[0].Head
	}
	d.y += 22
	d.text(svgMargin, d.y, "m", s)
}

// grid draws g as a heatmap.
// The order is column labels, then rows with labels, cells, and totals, then the legend.
func (d *svgDoc) grid(g axis.Grid, o Options) {
	if g.Total == 0 {
		d.note("no commits")
		return
	}
	v := visibleRows(g, o.MinRowTotal)
	if len(v.rows) == 0 {
		d.note(fmt.Sprintf("no %s with at least %d commits (%d hidden)",
			rowsName(g.Y), o.MinRowTotal, v.hidden))
		return
	}

	shown := make([]string, len(v.rows))
	for i, iy := range v.rows {
		shown[i] = g.Y.Label(iy)
	}
	labelPx := min(maxWidth(shown)*charPx+8, maxLabelPx)
	totalPx := len(strconv.Itoa(v.maxTotal))*charPx + 8
	nx := g.X.Len()
	colPx := min(max((svgWidth-2*svgMargin-labelPx-totalPx)/nx, 1), maxCellPx)
	rowPx := min(max(colPx, minRowPx), maxRowPx)
	gap := 0
	if colPx >= 8 {
		gap = 2
	}
	x0 := svgMargin + labelPx

	// Column labels. Skip some labels when columns are too narrow for all of them.
	every := max(1, (maxWidth(labels(g.X))*charPx+4+colPx-1)/colPx)
	d.y += 16
	for ix := 0; ix < nx; ix += every {
		d.text(x0+ix*colPx+(colPx-gap)/2, d.y, "m c", g.X.Label(ix))
	}
	d.y += 6

	for _, iy := range v.rows {
		label := g.Y.Label(iy)
		baseline := d.y + (rowPx-gap)/2 + 4
		d.text(svgMargin, baseline, "", Truncate(label, (labelPx-8)/charPx))
		for ix, n := range g.Cells[iy] {
			class := cellClass(o.Scale.Level(n, v.peak))
			title := strings.TrimSpace(label+" "+g.X.Label(ix)) + ": " + commits(n)
			d.rect(x0+ix*colPx, d.y, colPx-gap, rowPx-gap, class, title)
		}
		d.text(x0+nx*colPx+totalPx, baseline, "m e", strconv.Itoa(v.totals[iy]))
		d.y += rowPx
	}

	// A shaded chart without a legend is easy to misread.
	// Always write the scale, the maximum, and the lower bound of each level.
	d.y += 8
	d.note(fmt.Sprintf("%s scale · max %d at %s · empty cells = 0",
		o.Scale, v.peak, peakLabel(g, v.peakY, v.peakX)))
	steps := o.Scale.Steps(v.peak)
	d.legend("", "d", steps)
	if v.hidden > 0 {
		d.note(fmt.Sprintf("%d of %d %s hidden: fewer than %d commits",
			v.hidden, len(v.totals), rowsName(g.Y), o.MinRowTotal))
	}
}

// legend draws one line with a swatch for each level and the smallest count it stands for.
func (d *svgDoc) legend(label, prefix string, steps []Step) {
	d.y += lineStep + 2
	x := svgMargin
	if label != "" {
		d.text(x, d.y, "m", label)
		x += 140
	}
	for _, s := range steps {
		d.rect(x, d.y-swatchPx+2, swatchPx, swatchPx, prefix+strconv.Itoa(s.Level), "")
		n := strconv.Itoa(s.Min) + "+"
		d.text(x+swatchPx+4, d.y, "m", n)
		x += swatchPx + 4 + len(n)*charPx + 16
	}
}

// week draws hourly commits on weekdays and weekends as bars on each side of the hour column.
// Both sides share one scale for the same reason as the terminal Week view.
func (d *svgDoc) week(wk stats.Week) {
	total := wk.Total()
	if total == 0 {
		d.note("no commits")
		return
	}
	peak := max(slices.Max(wk.OnWeekdays[:]), slices.Max(wk.OnWeekend[:]))
	center, half := svgWidth/2, weekHourPx/2
	barMax := center - half - svgMargin - (len(strconv.Itoa(peak))*charPx + 8)

	days := wk.Weekend.Days()
	d.y += 16
	d.text(center-half, d.y, "m e", fmt.Sprintf("weekdays (%d days)", len(wk.Weekend)-days))
	d.text(center+half, d.y, "m", fmt.Sprintf("weekend: %s (%d days)",
		strings.Join(wk.Weekend.Names(), ", "), days))
	d.y += 6

	for h := range len(wk.OnWeekdays) {
		baseline := d.y + weekRowPx/2 + 3
		d.text(center, baseline, "m c", fmt.Sprintf("%02d", h))
		if n := wk.OnWeekdays[h]; n > 0 {
			px := barPx(n, peak, barMax)
			d.rect(center-half-px, d.y, px, weekRowPx-3, "d3",
				fmt.Sprintf("weekdays %02d:00: %s", h, commits(n)))
			d.text(center-half-px-4, baseline, "m e", strconv.Itoa(n))
		}
		if n := wk.OnWeekend[h]; n > 0 {
			px := barPx(n, peak, barMax)
			d.rect(center+half, d.y, px, weekRowPx-3, "d3",
				fmt.Sprintf("weekend %02d:00: %s", h, commits(n)))
			d.text(center+half+px+4, baseline, "m", strconv.Itoa(n))
		}
		d.y += weekRowPx
	}

	d.y += 8
	d.note(fmt.Sprintf("both sides share one scale (max %d) · weekdays %d (%s) · weekend %d (%s)",
		peak, wk.WeekdayTotal(), percent(wk.WeekdayTotal(), total),
		wk.WeekendTotal(), percent(wk.WeekendTotal(), total)))
	if idx, ok := wk.WeekendIndex(); ok {
		d.note(fmt.Sprintf("weekend index %.2f (1.00 = as busy per day as weekdays)", idx))
	} else {
		d.note("weekend index -")
	}
}

// palette is an SVG color scheme.
// The shading colors come from the same xterm-256 ramps as the terminal, so both look alike.
type palette struct {
	paper, ink, muted, rule, empty string
	shade                          ramp
}

var (
	lightPalette = palette{
		paper: "#faf8f4", ink: "#1f2328", muted: "#57606a", rule: "#c9cdd4", empty: "#ece8e1",
		shade: ramps[Light],
	}
	darkPalette = palette{
		paper: "#1f2328", ink: "#f0f3f6", muted: "#9da5ae", rule: "#3d444d", empty: "#2d333b",
		shade: ramps[Dark],
	}
)

func (p palette) css() string {
	var b strings.Builder
	fmt.Fprintf(&b, "text{fill:%s}.m{fill:%s}.bg{fill:%s}.rule{stroke:%s}.z{fill:%s}\n",
		p.ink, p.muted, p.paper, p.rule, p.empty)
	for i, c := range p.shade {
		fmt.Fprintf(&b, ".d%d{fill:%s}", i+1, xtermHex(c))
	}
	b.WriteString("\n")
	return b.String()
}

// SVG writes one SVG with sections stacked from top to bottom.
// The shared header and footer are drawn once.
//
// The SVG is a static figure for READMEs and slides.
// It does not rely on hover, so the scale, the maximum, and the legend are part of the image.
// The <title> of each cell becomes a tooltip only in a browser.
//
// The output follows these rules.
//   - The root has only a viewBox, so it follows the width of the page it is embedded in.
//   - No fonts, CSS, or images are loaded from outside.
//   - Run conditions go in <metadata>. XML comments cannot contain "--",
//     and a recorded command line such as --no-cache would break them.
//   - The same input gives the same bytes. No timestamp is written.
func SVG(w io.Writer, agg *model.Aggregate, sections []Section, o Options) error {
	d := &svgDoc{y: svgMargin}
	d.header(agg)
	titles := make([]string, len(sections))
	for i, s := range sections {
		titles[i] = s.Title
		d.rule()
		d.heading(s.Title)
		switch {
		case s.Grid != nil:
			d.grid(*s.Grid, o)
		case s.Week != nil:
			d.week(*s.Week)
		}
	}
	d.rule()
	d.footer(agg)
	height := d.y + svgMargin

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" role="img" `+
		`aria-labelledby="title desc">`+"\n", svgWidth, height)
	fmt.Fprintf(&b, "<title id=\"title\">%s</title>\n",
		xmlText("git-when: "+strings.Join(titles, ", ")))
	fmt.Fprintf(&b, "<desc id=\"desc\">%s</desc>\n", xmlText(summaryLine(agg)))
	b.WriteString("<metadata>\n")
	for _, l := range append(metaLines(agg), repoLines(agg)...) {
		b.WriteString(xmlText(l) + "\n")
	}
	b.WriteString("</metadata>\n")
	fmt.Fprintf(&b, "<style>\n%s</style>\n", svgStyle(o.SVGTheme))
	fmt.Fprintf(&b, "<rect class=\"bg\" width=\"%d\" height=\"%d\"/>\n", svgWidth, height)
	b.WriteString(d.b.String())
	b.WriteString("</svg>\n")

	_, err := io.WriteString(w, b.String())
	return err
}

// summaryLine states the basis of the figure in one line.
// It always names the time basis, the period, and the counts.
func summaryLine(agg *model.Aggregate) string {
	return fmt.Sprintf("author local time · %s · %s · %d repositories",
		Period(agg), commits(agg.Total()), len(agg.Projects))
}

// barPx returns the bar length for n, up to maxPx, relative to peak.
// Any value above zero is at least 1px long.
func barPx(n, peak, maxPx int) int {
	if n <= 0 || peak <= 0 {
		return 0
	}
	return max(n*maxPx/peak, 1)
}

// cellClass returns the CSS class of a cell with the given level. Zero commits use "z".
func cellClass(level int) string {
	if level <= 0 {
		return "z"
	}
	return "d" + strconv.Itoa(level)
}

// commits writes a count such as "1 commit" or "2 commits".
func commits(n int) string {
	if n == 1 {
		return "1 commit"
	}
	return strconv.Itoa(n) + " commits"
}

func svgStyle(t SVGTheme) string {
	switch t {
	case SVGLight:
		return svgFont + lightPalette.css()
	case SVGDark:
		return svgFont + darkPalette.css()
	default:
		return svgFont + lightPalette.css() +
			"@media (prefers-color-scheme:dark){\n" + darkPalette.css() + "}\n"
	}
}

// xtermHex converts an xterm-256 color code (16-255) to #rrggbb.
func xtermHex(code int) string {
	if code >= 232 { // The 24 gray steps.
		v := 8 + (code-232)*10
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	}
	cube := [6]int{0, 95, 135, 175, 215, 255} // Steps of the 6x6x6 color cube.
	c := max(code-16, 0)
	return fmt.Sprintf("#%02x%02x%02x", cube[c/36%6], cube[c/6%6], cube[c%6])
}

// xmlText makes s safe for XML text and attribute values.
// Author names can be any string.
// Control characters that XML 1.0 forbids and invalid UTF-8 become U+FFFD.
func xmlText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	s = strings.Map(func(r rune) rune {
		if (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0xFFFE || r == 0xFFFF {
			return '�'
		}
		return r
	}, s)
	return xmlEscaper.Replace(s)
}
