package collect

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tsutsu3/git-when/internal/cache"
	"github.com/tsutsu3/git-when/internal/gittest"
	"github.com/tsutsu3/git-when/internal/model"
)

const (
	alice = "Alice"
	bob   = "Bob"
)

func TestNewFilterRejectsInvalidRegexp(t *testing.T) {
	for _, o := range []FilterOptions{
		{Author: "("},
		{ExcludeAuthor: "[a-"},
	} {
		if _, err := NewFilter(o); err == nil {
			t.Errorf("NewFilter(%+v) succeeded, want error", o)
		}
	}
}

func TestFilterMatch(t *testing.T) {
	commits := []Commit{
		{Name: alice, Email: "alice@example.com"},
		{Name: bob, Email: "bob@corp.example"},
		{Name: "dependabot[bot]", Email: "support@github.com"},
		{Name: "Renovate Bot", Email: "bot@renovateapp.com"},
	}
	cases := []struct {
		name string
		opts FilterOptions
		want []string
	}{
		{"default excludes bots", FilterOptions{}, []string{alice, bob}},
		{"author by email", FilterOptions{Author: "alice@"}, []string{alice}},
		{"author by name, case-insensitive", FilterOptions{Author: "(?i)^bob "}, []string{bob}},
		{"author by domain", FilterOptions{Author: `@corp\.example>$`}, []string{bob}},
		{"exclude author", FilterOptions{ExcludeAuthor: bob}, []string{alice}},
		{
			"bots restores bots",
			FilterOptions{Bots: true},
			[]string{alice, bob, "dependabot[bot]", "Renovate Bot"},
		},
		{"bot exclusion wins over author", FilterOptions{Author: "dependabot"}, nil},
		{
			"author with bots",
			FilterOptions{Author: "dependabot", Bots: true},
			[]string{"dependabot[bot]"},
		},
		{"no match", FilterOptions{Author: "carol"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := NewFilter(c.opts)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, cm := range commits {
				if f.Match(cm) {
					got = append(got, cm.Name)
				}
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestCollect checks collection against real repositories.
func TestCollect(t *testing.T) {
	dir := gittest.New(t,
		gittest.CommitBy(alice, "alice@example.com", "2024-03-05T09:00:00+09:00"),
		gittest.CommitBy(alice, "alice@example.com", "2024-03-06T22:00:00+09:00"),
		gittest.CommitBy(bob, "bob@example.com", "2024-03-05T10:00:00-08:00"),
		gittest.CommitBy("dependabot[bot]", "support@github.com", "2024-03-07T03:00:00Z"),
	)

	cases := []struct {
		name        string
		opts        FilterOptions
		wantN       int
		wantAuthors []string
	}{
		{"default excludes bots", FilterOptions{}, 3, []string{alice, bob}},
		{"author picks one of two", FilterOptions{Author: "alice@"}, 2, []string{alice}},
		{
			"bots restores bots",
			FilterOptions{Bots: true},
			4,
			[]string{alice, bob, "dependabot[bot]"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := NewFilter(c.opts)
			if err != nil {
				t.Fatal(err)
			}
			agg := model.New()
			res, err := (&Collector{Filter: f}).Collect(t.Context(), agg, 0, dir)
			if err != nil {
				t.Fatal(err)
			}
			if res.Commits != c.wantN || agg.Total() != c.wantN {
				t.Errorf("Collect = %d, Total = %d, want %d", res.Commits, agg.Total(), c.wantN)
			}

			var names []string
			for _, au := range agg.Authors {
				names = append(names, au.Name)
			}
			slices.Sort(names)
			if !slices.Equal(names, c.wantAuthors) {
				t.Errorf("authors = %q, want %q", names, c.wantAuthors)
			}

			// Author indexes in buckets point into Authors.
			for _, u := range agg.SortedKeys() {
				if k := model.Unpack(u); int(k.Author) >= len(agg.Authors) {
					t.Errorf("bucket %+v points outside Authors (len %d)", k, len(agg.Authors))
				}
			}
		})
	}
}

func TestCollectReportsGitError(t *testing.T) {
	f, err := NewFilter(FilterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&Collector{Filter: f}).Collect(
		t.Context(),
		model.New(),
		0,
		t.TempDir(),
	); err == nil {
		t.Error("Collect on a non-repository succeeded, want error")
	}
}

// TestCollectCache checks that git log output is read from the cache,
// and read again when .mailmap or HEAD changes.
func TestCollectCache(t *testing.T) {
	dir := gittest.New(t, gittest.CommitBy(alice, "alice@example.com", "2024-03-05T09:00:00+09:00"))
	c := cache.New(t.TempDir())
	f, err := NewFilter(FilterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	authors := func() []string {
		t.Helper()
		agg := model.New()
		if _, err := (&Collector{Filter: f, Cache: c}).Collect(
			t.Context(),
			agg,
			0,
			dir,
		); err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, au := range agg.Authors {
			names = append(names, au.Name)
		}
		slices.Sort(names)
		return names
	}

	if got := authors(); !slices.Equal(got, []string{alice}) {
		t.Fatalf("first Collect = %q, want [Alice]", got)
	}

	// Replace the entry from the first run with content that git log would not print.
	// If the second run returns that content, it read the cache instead of running git log.
	key := cacheKey(dir, revParseHead(t.Context(), dir))
	if _, hit := c.Get(key); !hit {
		t.Fatal("first Collect did not store the log")
	}
	fake := "2024-03-06T10:00:00+09:00\tFrom Cache\tcache@example.com"
	if err := c.Put(key, []byte(fake)); err != nil {
		t.Fatal(err)
	}
	if got := authors(); !slices.Equal(got, []string{"From Cache"}) {
		t.Errorf("second Collect = %q, want the cached log", got)
	}

	// A new .mailmap changes identities, so the log is read again.
	mailmap := []byte("Alice Liddell <alice@example.com>\n")
	if err := os.WriteFile(filepath.Join(dir, ".mailmap"), mailmap, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := authors(); !slices.Equal(got, []string{"Alice Liddell"}) {
		t.Errorf("after .mailmap changed = %q, want [Alice Liddell]", got)
	}

	// A new commit changes HEAD, so the log is read again.
	gittest.Run(t, dir, []string{
		"GIT_AUTHOR_NAME=" + bob, "GIT_AUTHOR_EMAIL=bob@example.com",
		"GIT_COMMITTER_NAME=" + bob, "GIT_COMMITTER_EMAIL=bob@example.com",
	}, "commit", "--quiet", "--allow-empty", "-m", "bob")
	if got := authors(); !slices.Equal(got, []string{"Alice Liddell", bob}) {
		t.Errorf("after a new commit = %q, want [Alice Liddell Bob]", got)
	}
}

func TestCollectCacheSkipsEmptyRepository(t *testing.T) {
	f, err := NewFilter(FilterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := t.TempDir()
	res, err := (&Collector{Filter: f, Cache: cache.New(cacheDir)}).Collect(
		t.Context(),
		model.New(),
		0,
		gittest.New(t),
	)
	if res != (Result{}) || err != nil {
		t.Errorf("Collect on an empty repository = %+v, %v; want zero, nil", res, err)
	}
	if entries, err := os.ReadDir(cacheDir); err != nil || len(entries) != 0 {
		t.Errorf("cache has %v (%v), want nothing for a repository without HEAD", entries, err)
	}
}

func TestCollectorTimings(t *testing.T) {
	dir := gittest.New(t, gittest.CommitAt("2024-03-05T09:00:00+09:00"))

	// A nil Filter keeps every commit. The first run misses and the next two hit the cache.
	col := &Collector{Cache: cache.New(t.TempDir())}
	for range 3 {
		if res, err := col.Collect(
			t.Context(),
			model.New(),
			0,
			dir,
		); err != nil ||
			res.Commits != 1 {
			t.Fatalf("Collect = %+v, %v; want 1 commit, nil", res, err)
		}
	}
	if tm := col.Timings; tm.CacheMisses != 1 || tm.CacheHits != 2 || tm.Git <= 0 {
		t.Errorf("Timings = %+v; want 1 miss, 2 hits and some time in git", tm)
	}

	// Without a cache, no hits or misses are counted and git log runs every time.
	plain := &Collector{}
	if _, err := plain.Collect(t.Context(), model.New(), 0, dir); err != nil {
		t.Fatal(err)
	}
	if tm := plain.Timings; tm.CacheHits+tm.CacheMisses != 0 || tm.Git <= 0 || tm.Cache != 0 {
		t.Errorf("Timings without a cache = %+v", tm)
	}
}

// TestCollectHead checks that Collect returns HEAD at read time, which CSV records.
// It checks both a git log run and a cache read.
func TestCollectHead(t *testing.T) {
	dir := gittest.New(t, gittest.CommitAt("2024-03-05T09:00:00+09:00"))
	want := strings.TrimSpace(gittest.Run(t, dir, nil, "rev-parse", "HEAD"))
	if want == "" {
		t.Fatal("rev-parse HEAD printed nothing")
	}

	col := &Collector{Cache: cache.New(t.TempDir())}
	for _, from := range []string{"git log", "the cache"} {
		res, err := col.Collect(t.Context(), model.New(), 0, dir)
		if err != nil || res.Head != want {
			t.Errorf("Collect from %s = %+v, %v; want head %s", from, res, err, want)
		}
	}
	if col.Timings.CacheHits != 1 {
		t.Errorf("Timings = %+v, want the second Collect to hit the cache", col.Timings)
	}

	res, err := (&Collector{}).Collect(t.Context(), model.New(), 0, gittest.New(t))
	if err != nil || res.Head != "" {
		t.Errorf("Collect on an empty repository = %+v, %v; want no head", res, err)
	}
}

func TestLogEmptyRepository(t *testing.T) {
	// git log fails right after git init. The repository is not broken, so it has zero commits.
	repos := map[string]string{"worktree": gittest.New(t), "bare": gittest.NewBare(t)}
	for name, dir := range repos {
		t.Run(name, func(t *testing.T) {
			commits, err := Log(t.Context(), dir)
			if err != nil || len(commits) != 0 {
				t.Errorf("Log = %+v, %v; want no commits and no error", commits, err)
			}
		})
	}
}

func TestLog(t *testing.T) {
	type want struct {
		hour    int
		offset  int // Seconds.
		weekday time.Weekday
		day     int
		name    string
	}
	cases := []struct {
		name   string
		commit gittest.Commit
		want   want
	}{
		{
			"JST",
			gittest.CommitAt("2024-03-05T09:00:00+09:00"),
			want{9, 9 * 3600, time.Tuesday, 5, gittest.DefaultName},
		},
		// Tuesday 05 in UTC.
		{
			"PST",
			gittest.CommitAt("2024-03-04T21:00:00-08:00"),
			want{21, -8 * 3600, time.Monday, 4, gittest.DefaultName},
		},
		{
			"Nepal +05:45",
			gittest.CommitAt("2024-03-05T14:15:00+05:45"),
			want{14, 5*3600 + 45*60, time.Tuesday, 5, gittest.DefaultName},
		},
		{
			"midnight",
			gittest.CommitAt("2024-03-06T00:00:00+09:00"),
			want{0, 9 * 3600, time.Wednesday, 6, gittest.DefaultName},
		},
		{
			"last second",
			gittest.CommitAt("2024-03-06T23:59:59+09:00"),
			want{23, 9 * 3600, time.Wednesday, 6, gittest.DefaultName},
		},
		{
			"leap day",
			gittest.CommitAt("2024-02-29T12:00:00+09:00"),
			want{12, 9 * 3600, time.Thursday, 29, gittest.DefaultName},
		},
		// Sunday 23 in UTC, but Monday 08 in author local time.
		{
			"crosses date in UTC",
			gittest.CommitAt("2024-03-11T08:00:00+09:00"),
			want{8, 9 * 3600, time.Monday, 11, gittest.DefaultName},
		},
		{
			"bot",
			gittest.CommitBy("dependabot[bot]", "support@github.com", "2024-03-07T03:00:00Z"),
			want{3, 0, time.Thursday, 7, "dependabot[bot]"},
		},
		// The author date is used, not the committer date.
		{
			"rebased later",
			gittest.Commit{
				AuthorDate:    "2024-03-09T21:00:00+09:00",
				CommitterDate: "2024-03-12T10:00:00Z",
			},
			want{21, 9 * 3600, time.Saturday, 9, gittest.DefaultName},
		},
	}

	commits := make([]gittest.Commit, len(cases))
	byInstant := map[int64]want{}
	for i, c := range cases {
		commits[i] = c.commit
		byInstant[mustTime(t, c.commit.AuthorDate).Unix()] = c.want
	}
	if len(byInstant) != len(cases) {
		t.Fatal("test cases must have distinct author instants")
	}

	repos := map[string]string{
		"worktree": gittest.New(t, commits...),
		"bare":     gittest.NewBare(t, commits...),
	}
	for name, dir := range repos {
		t.Run(name, func(t *testing.T) {
			got, err := Log(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(cases) {
				t.Fatalf("got %d commits, want %d", len(got), len(cases))
			}
			for _, c := range got {
				w, ok := byInstant[c.Time.Unix()]
				if !ok {
					t.Errorf("unexpected commit at %s", c.Time.Format(time.RFC3339))
					continue
				}
				_, offset := c.Time.Zone()
				g := want{c.Time.Hour(), offset, c.Time.Weekday(), c.Time.Day(), c.Name}
				if g != w {
					t.Errorf("%s: got %+v, want %+v", c.Time.Format(time.RFC3339), g, w)
				}
			}
		})
	}
}

func TestLogUsesMailmap(t *testing.T) {
	// One person commits with work and personal emails and different spellings of the name.
	dir := gittest.New(t,
		gittest.CommitBy("Taro Yamada", "taro@work.example", "2024-03-05T09:00:00+09:00"),
		gittest.CommitBy("taro", "taro@home.example", "2024-03-09T23:00:00+09:00"),
	)
	mailmap := "Taro Yamada <taro@work.example>\n" +
		"Taro Yamada <taro@work.example> <taro@home.example>\n"
	if err := os.WriteFile(filepath.Join(dir, ".mailmap"), []byte(mailmap), 0o600); err != nil {
		t.Fatal(err)
	}

	commits, err := Log(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range commits {
		if got, want := c.Ident(), "Taro Yamada <taro@work.example>"; got != want {
			t.Errorf("Ident = %q, want %q", got, want)
		}
	}
}

func TestParseLog(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want []Commit
	}{
		{"empty", "", nil},
		{
			"offset kept",
			"2024-03-05T09:00:00+09:00\tAlice\talice@example.com",
			[]Commit{{mustTime(t, "2024-03-05T09:00:00+09:00"), alice, "alice@example.com"}},
		},
		{
			"UTC as Z, trailing newline",
			"2024-03-07T03:00:00Z\tdependabot[bot]\tsupport@github.com\n",
			[]Commit{
				{mustTime(t, "2024-03-07T03:00:00Z"), "dependabot[bot]", "support@github.com"},
			},
		},
		{
			"name with spaces and tab",
			"2024-03-05T09:00:00+09:00\tTaro\tYamada Jr\ttaro@example.com",
			[]Commit{
				{mustTime(t, "2024-03-05T09:00:00+09:00"), "Taro\tYamada Jr", "taro@example.com"},
			},
		},
		{
			"empty email",
			"2024-03-05T09:00:00+09:00\tAlice\t",
			[]Commit{{mustTime(t, "2024-03-05T09:00:00+09:00"), alice, ""}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseLog([]byte(c.out))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.EqualFunc(got, c.want, sameCommit) {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestParseLogRejectsUnexpectedFormat(t *testing.T) {
	for _, out := range []string{
		"2024-03-05T09:00:00+09:00",                     // No author (wrong format).
		"2024-03-05T09:00:00+09:00\tAlice",              // Only one tab.
		"2024-03-05 09:00:00 +0900\tAlice\ta@example",   // The %ai format.
		"2024-03-05T09:00:00+09:00\tA\ta\nnot a commit", // The second line is broken.
	} {
		if got, err := ParseLog([]byte(out)); err == nil {
			t.Errorf("ParseLog(%q) = %+v, want error", out, got)
		}
	}
}

func TestIsBot(t *testing.T) {
	bots := []string{
		"dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>",
		"github-actions[bot] <41898282+github-actions[bot]@users.noreply.github.com>",
		"renovate[bot] <29139614+renovate[bot]@users.noreply.github.com>",
		"Renovate Bot <bot@renovateapp.com>",
		"renovate <renovate@whitesourcesoftware.com>",
		"Dependabot <support@github.com>",
	}
	humans := []string{
		"Alice <alice@example.com>",
		"Abbott <abbott@example.com>",
		"Jane Talbot <jane@example.com>",
		"Robot Fan <robot@example.com>",
		"Bottle <bottle@example.com>",
		// A human author with GitHub as the committer.
		"Alice <alice@users.noreply.github.com>",
	}
	for _, id := range bots {
		if !IsBot(id) {
			t.Errorf("IsBot(%q) = false, want true", id)
		}
	}
	for _, id := range humans {
		if IsBot(id) {
			t.Errorf("IsBot(%q) = true, want false", id)
		}
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return tm
}

// sameCommit compares times by instant and offset.
// == on time.Time also compares Location pointers.
func sameCommit(a, b Commit) bool {
	_, ao := a.Time.Zone()
	_, bo := b.Time.Zone()
	return a.Time.Equal(b.Time) && ao == bo && a.Name == b.Name && a.Email == b.Email
}
