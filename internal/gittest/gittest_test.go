package gittest

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNewPreservesDatesAndAuthors(t *testing.T) {
	commits := []Commit{
		CommitAt("2024-03-05T09:00:00+09:00"),
		CommitAt("2024-03-04T18:30:00-08:00"),
		CommitAt("2024-03-05T15:15:00+05:45"),
		CommitAt("2024-02-29T00:00:00+09:00"),
		{AuthorDate: "2024-03-06T23:59:59+09:00", CommitterDate: "2024-03-07T10:00:00Z"},
		CommitBy("dependabot[bot]", "support@github.com", "2024-03-07T03:00:00Z"),
	}
	dir := New(t, commits...)

	var want []string
	for _, c := range commits {
		cd := c.CommitterDate
		if cd == "" {
			cd = c.AuthorDate
		}
		name, email := c.Name, c.Email
		if name == "" {
			name, email = DefaultName, DefaultEmail
		}
		want = append(want, strings.Join(
			[]string{
				normalize(t, c.AuthorDate),
				normalize(t, cd),
				name + " <" + email + ">",
			},
			"\t",
		))
	}

	var got []string
	// The output is AuthorDate, CommitterDate, and AuthorName <AuthorEmail>, in that order.
	for _, l := range lines(Run(t, dir, nil, "log", "--format=%aI%x09%cI%x09%aN <%aE>")) {
		f := strings.SplitN(l, "\t", 3)
		if len(f) != 3 {
			t.Fatalf("unexpected git log line %q", l)
		}
		got = append(
			got,
			strings.Join([]string{normalize(t, f[0]), normalize(t, f[1]), f[2]}, "\t"),
		)
	}

	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("git log mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestNewBare(t *testing.T) {
	dir := NewBare(t, CommitAt("2024-03-05T09:00:00+09:00"), CommitAt("2024-03-05T10:00:00+09:00"))

	if got := strings.TrimSpace(
		Run(t, dir, nil, "rev-parse", "--is-bare-repository"),
	); got != "true" {
		t.Errorf("is-bare-repository = %q, want true", got)
	}
	if got := strings.TrimSpace(Run(t, dir, nil, "rev-list", "--count", "HEAD")); got != "2" {
		t.Errorf("commits reachable from HEAD = %s, want 2", got)
	}
}

func TestEmpty(t *testing.T) {
	for name, dir := range map[string]string{"worktree": New(t), "bare": NewBare(t)} {
		t.Run(name, func(t *testing.T) {
			if got := strings.TrimSpace(
				Run(t, dir, nil, "rev-list", "--all", "--count"),
			); got != "0" {
				t.Errorf("commits = %s, want 0", got)
			}
		})
	}
}

func TestIsolatedFromCallerEnvironment(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "commit.gpgsign")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "elsewhere"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "missing", "index"))

	dir := New(t, CommitAt("2024-03-05T09:00:00+09:00"))

	got := strings.TrimSpace(Run(t, dir, nil, "log", "--format=%aI %aN"))
	if want := "2024-03-05T09:00:00+09:00 " + DefaultName; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// normalize rewrites an RFC 3339 date into a form for comparison.
// Git prints UTC as Z or +00:00 depending on the version, so raw strings are not compared.
func normalize(t *testing.T, s string) string {
	t.Helper()
	d, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return d.Format("2006-01-02T15:04:05-07:00")
}

func lines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}
