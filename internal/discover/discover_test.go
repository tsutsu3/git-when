package discover

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tsutsu3/git-when/internal/gittest"
)

var commit = gittest.CommitAt("2024-03-05T09:00:00+09:00")

func TestDiscover(t *testing.T) {
	root := tempDir(t)
	place(t, gittest.New(t, commit), root, "a")
	// Repositories inside a repository, such as submodules, are not counted.
	place(t, gittest.New(t, commit), root, "a/sub")
	place(t, gittest.New(t, commit), root, "b/c")
	place(t, gittest.NewBare(t, commit), root, "d/e/f.git")
	// Dependency directories are not searched.
	place(t, gittest.New(t, commit), root, "node_modules/pkg")
	place(t, gittest.New(t, commit), root, "vendor/lib")
	// A .git file without gitdir is not a repository, so the search goes inside.
	writeFile(t, root, "notrepo/.git", "not a gitdir pointer\n")
	place(t, gittest.New(t, commit), root, "notrepo/inner")
	// A submodule-style .git file.
	writeFile(t, root, "s/mod/.git", "gitdir: ../../a/.git/modules/mod\n")
	// A worktree .git file.
	gittest.Run(t, filepath.Join(root, "a"), nil,
		"worktree", "add", "--quiet", filepath.Join(root, "w", "tree"))
	writeFile(t, root, "plain/README", "no repository here\n")

	repos, err := Discover([]string{root}, Options{})
	if err != nil {
		t.Fatal(err)
	}

	want := []Repo{
		{Path: filepath.Join(root, "a"), Name: "a"},
		{Path: filepath.Join(root, "b", "c"), Name: "b/c"},
		{Path: filepath.Join(root, "d", "e", "f.git"), Name: "d/e/f.git", Bare: true},
		{Path: filepath.Join(root, "notrepo", "inner"), Name: "notrepo/inner"},
		{Path: filepath.Join(root, "s", "mod"), Name: "s/mod"},
		{Path: filepath.Join(root, "w", "tree"), Name: "w/tree"},
	}
	if !slices.Equal(repos, want) {
		t.Errorf("Discover =\n%+v\nwant\n%+v", repos, want)
	}
}

func TestDiscoverRootIsRepository(t *testing.T) {
	root := evalSymlinks(t, gittest.New(t, commit))
	place(t, gittest.New(t, commit), root, "sub")

	repos, err := Discover([]string{root}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := []Repo{{Path: root, Name: filepath.Base(root)}}
	if !slices.Equal(repos, want) {
		t.Errorf("Discover = %+v, want %+v", repos, want)
	}
}

func TestDiscoverMaxDepth(t *testing.T) {
	root := tempDir(t)
	place(t, gittest.New(t, commit), root, "top")        // Depth 1.
	place(t, gittest.New(t, commit), root, "l1/l2/deep") // Depth 3.

	cases := []struct {
		maxDepth int
		want     []string
	}{
		{0, []string{"l1/l2/deep", "top"}}, // No limit.
		{3, []string{"l1/l2/deep", "top"}},
		{2, []string{"top"}},
		{1, []string{"top"}},
	}
	for _, c := range cases {
		got := discoverNames(t, []string{root}, Options{MaxDepth: c.maxDepth})
		if !slices.Equal(got, c.want) {
			t.Errorf("MaxDepth %d: got %q, want %q", c.maxDepth, got, c.want)
		}
	}
}

func TestDiscoverSkipDirs(t *testing.T) {
	root := tempDir(t)
	place(t, gittest.New(t, commit), root, "node_modules/pkg")
	place(t, gittest.New(t, commit), root, "cache/repo")

	if got := discoverNames(t, []string{root}, Options{}); len(got) != 1 || got[0] != "cache/repo" {
		t.Errorf("default SkipDirs: got %q", got)
	}
	got := discoverNames(t, []string{root}, Options{SkipDirs: []string{"cache"}})
	if want := []string{"node_modules/pkg"}; !slices.Equal(got, want) {
		t.Errorf("custom SkipDirs: got %q, want %q", got, want)
	}

	// A directory given as a root is searched even when its name is on the skip list.
	got = discoverNames(t, []string{filepath.Join(root, "node_modules")}, Options{})
	if want := []string{"pkg"}; !slices.Equal(got, want) {
		t.Errorf("skip-named root: got %q, want %q", got, want)
	}
}

func TestDiscoverSymlinks(t *testing.T) {
	outside := gittest.New(t, commit)
	root := tempDir(t)
	place(t, gittest.New(t, commit), root, "real")
	// A link to an outside repository is not followed.
	symlink(t, outside, root, "link")
	// A link to itself does not loop forever.
	symlink(t, root, root, "loop")
	// A broken link is not an error.
	symlink(t, filepath.Join(root, "missing"), root, "broken")

	got := discoverNames(t, []string{root}, Options{})
	if want := []string{"real"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}

	// With FollowSymlinks, a link to an outside directory is searched under the link name.
	ext := tempDir(t)
	place(t, gittest.New(t, commit), ext, "app")
	symlink(t, ext, root, "ext")
	// A link back to the root would loop, so it is skipped.
	symlink(t, root, ext, "back")
	// The walk reaches a target inside the root directly, so "again" does not take the name of "real".
	symlink(t, filepath.Join(root, "real"), root, "again")
	// The target of "link" is already searched, so "link2" does not count it twice.
	symlink(t, outside, root, "link2")
	follow := Options{FollowSymlinks: true}
	got = discoverNames(t, []string{root}, follow)
	if want := []string{"ext/app", "link", "real"}; !slices.Equal(got, want) {
		t.Errorf("follow: got %q, want %q", got, want)
	}

	// Depth counts the levels through a link. "ext" is at depth 1, so ext/app is too deep.
	follow.MaxDepth = 1
	got = discoverNames(t, []string{root}, follow)
	if want := []string{"link", "real"}; !slices.Equal(got, want) {
		t.Errorf("follow with max depth: got %q, want %q", got, want)
	}

	// A root that is a link is resolved before the search.
	linkRoot := filepath.Join(tempDir(t), "linkroot")
	if err := os.Symlink(root, linkRoot); err != nil {
		t.Fatal(err)
	}
	repos, err := Discover([]string{linkRoot}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := []Repo{{Path: filepath.Join(root, "real"), Name: "real"}}
	if !slices.Equal(repos, want) {
		t.Errorf("symlinked root: got %+v, want %+v", repos, want)
	}
}

func TestDiscoverSkipsUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := tempDir(t)
	place(t, gittest.New(t, commit), root, "locked/repo")
	place(t, gittest.New(t, commit), root, "open")
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	// Restore permissions so TempDir cleanup can remove it.
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) }) //nolint:gosec // Dirs need exec.

	var warned []string
	repos, err := Discover([]string{root}, Options{
		Warn: func(path string, _ error) { warned = append(warned, path) },
	})
	if err != nil {
		t.Fatalf("Discover failed instead of skipping: %v", err)
	}
	if got, want := names(repos), []string{"open"}; !slices.Equal(got, want) {
		t.Errorf("repos = %q, want %q", got, want)
	}
	if want := []string{locked}; !slices.Equal(warned, want) {
		t.Errorf("warned = %q, want %q", warned, want)
	}
}

func TestDiscoverOverlappingRoots(t *testing.T) {
	root := tempDir(t)
	place(t, gittest.New(t, commit), root, "b/c")

	repos, err := Discover([]string{root, filepath.Join(root, "b"), root}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// The name comes from the first root that finds it. With several roots it has the root base name.
	want := []Repo{{Path: filepath.Join(root, "b", "c"), Name: filepath.Base(root) + "/b/c"}}
	if !slices.Equal(repos, want) {
		t.Errorf("got %+v, want %+v", repos, want)
	}
}

func TestDiscoverMultipleRoots(t *testing.T) {
	parent := tempDir(t)
	work, home := filepath.Join(parent, "work"), filepath.Join(parent, "home")
	place(t, gittest.New(t, commit), work, "app")
	place(t, gittest.New(t, commit), home, "app")
	place(t, gittest.New(t, commit), home, "dotfiles/nvim")
	// A root that is itself a repository is named by its base name, even with several roots.
	solo := filepath.Join(parent, "solo")
	place(t, gittest.New(t, commit), parent, "solo")

	got := discoverNames(t, []string{work, home, solo}, Options{})
	want := []string{"work/app", "home/app", "home/dotfiles/nvim", "solo"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}

	// With one root, the name is the path relative to the root.
	got = discoverNames(t, []string{home}, Options{})
	if want := []string{"app", "dotfiles/nvim"}; !slices.Equal(got, want) {
		t.Errorf("single root: got %q, want %q", got, want)
	}
}

func TestDiscoverRootErrors(t *testing.T) {
	root := tempDir(t)
	writeFile(t, root, "file", "x")
	for _, bad := range []string{filepath.Join(root, "missing"), filepath.Join(root, "file")} {
		if repos, err := Discover([]string{bad}, Options{}); err == nil {
			t.Errorf("Discover(%q) = %+v, want error", bad, repos)
		}
	}
}

func TestDiscoverProgress(t *testing.T) {
	root := tempDir(t)
	place(t, gittest.New(t, commit), root, "a")
	place(t, gittest.New(t, commit), root, "b/c")
	place(t, gittest.New(t, commit), root, "node_modules/pkg") // Not entered, so not counted.
	writeFile(t, root, "plain/README", "x")

	type call struct {
		dirs, repos int
		path        string
	}
	var calls []call
	repos, err := Discover([]string{root}, Options{
		Progress: func(dirs, repos int, path string) { calls = append(calls, call{dirs, repos, path}) },
	})
	if err != nil {
		t.Fatal(err)
	}

	// root, a, b, b/c, and plain are visited. Repositories and node_modules are not entered.
	wantPaths := []string{
		root, filepath.Join(root, "a"), filepath.Join(root, "b"),
		filepath.Join(root, "b", "c"), filepath.Join(root, "plain"),
	}
	if len(calls) != len(wantPaths) {
		t.Fatalf("Progress called %d times, want %d: %+v", len(calls), len(wantPaths), calls)
	}
	for i, c := range calls {
		if c.dirs != i+1 || c.path != wantPaths[i] || c.repos > len(repos) {
			t.Errorf(
				"call %d = %+v, want dirs=%d path=%s repos<=%d",
				i,
				c,
				i+1,
				wantPaths[i],
				len(repos),
			)
		}
	}
	// By the time plain is visited, a and b/c have been found.
	if last := calls[len(calls)-1]; last.repos != 2 {
		t.Errorf("repos at the last directory = %d, want 2", last.repos)
	}
}

// tempDir returns a temporary directory with symbolic links resolved.
// Discover returns resolved paths, so the expected paths must match.
func tempDir(t *testing.T) string {
	t.Helper()
	return evalSymlinks(t, t.TempDir())
}

func evalSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// place moves the repository at src to root/rel.
func place(t *testing.T, src, root, rel string) {
	t.Helper()
	dst := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src, dst); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, root, rel string) {
	t.Helper()
	if err := os.Symlink(target, filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
}

func discoverNames(t *testing.T, roots []string, o Options) []string {
	t.Helper()
	repos, err := Discover(roots, o)
	if err != nil {
		t.Fatal(err)
	}
	return names(repos)
}

func names(repos []Repo) []string {
	out := make([]string, len(repos))
	for i, r := range repos {
		out[i] = r.Name
	}
	return out
}
