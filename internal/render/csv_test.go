package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/tsutsu3/git-when/internal/model"
)

func TestCSV(t *testing.T) {
	agg := model.New()
	agg.Projects = []model.Project{
		{Name: "work/api", Head: "abc123", Commits: 3},
		{Name: `#tmp,"x"`, Commits: 1}, // Starts with # and contains , and ".
		{Name: "empty"},
	}
	agg.Authors = []model.Author{
		{Name: "Bob", Email: "bob@example.com"},
		{Name: "Alice", Email: "alice@example.com"},
	}
	agg.Meta = model.Meta{
		Tool: "git-when", Version: "dev", TZ: "author",
		Command: "git-when -f csv\n~/src", // A newline must not split the # line.
		Filters: model.Filters{Author: "@example"},
	}
	const bob, alice = 0, 1
	for _, c := range []struct {
		author, project uint16
		time            string
	}{
		{alice, 0, "2024-03-05T09:00:00+09:00"}, // Tue 09
		{alice, 0, "2024-03-05T09:10:00+09:00"}, // Same bucket.
		{bob, 0, "2023-12-31T23:59:59-08:00"},   // Sun 23 in author local time.
		{bob, 1, "2024-03-04T09:30:00Z"},        // Mon 09
	} {
		tm, err := time.Parse(time.RFC3339, c.time)
		if err != nil {
			t.Fatal(err)
		}
		_, offset := tm.Zone()
		//nolint:gosec // Fixed test offsets are within ±26 hours.
		if err := agg.Add(c.author, c.project, tm, int32(offset)); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	if err := CSV(&buf, agg); err != nil {
		t.Fatal(err)
	}
	// Bob has the lower author index, but rows are grouped by repository.
	want := `# git-when dev
# command: "git-when -f csv\n~/src"
# time zone: author
# period: 2023-12 to 2024-03
# filters: author="@example", bots excluded, merges excluded
# month: 1=Jan .. 12=Dec; weekday: 1=Mon .. 7=Sun (ISO 8601); hour: 0-23 in author local time
# repository: head=abc123 commits=3 name=work/api
# repository: head=- commits=1 name=#tmp,"x"
# repository: head=- commits=0 name=empty
project,author_name,author_email,year,month,weekday,hour,commits
"work/api","Bob","bob@example.com",2023,12,7,23,1
"work/api","Alice","alice@example.com",2024,3,2,9,2
"#tmp,""x""","Bob","bob@example.com",2024,3,1,9,1
`
	if got := buf.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCSVWithoutCommits(t *testing.T) {
	var buf bytes.Buffer
	if err := CSV(&buf, model.New()); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if last := lines[len(lines)-1]; last != strings.Join(csvHeader, ",") {
		t.Errorf("last line = %q, want only the header after the # lines:\n%s", last, buf.String())
	}
	if !strings.Contains(buf.String(), "# period: -\n") {
		t.Errorf("period of an empty aggregate is not -:\n%s", buf.String())
	}
}
