// Package discover walks directories and finds Git repositories.
package discover

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// DefaultSkipDirs lists directory names that are not entered by default.
// They hold dependencies or build output, not the user's own commits.
var DefaultSkipDirs = []string{
	"node_modules", "bower_components", "vendor", "target",
	".venv", "venv", "__pycache__", ".tox", ".terraform",
}

// Repo is one repository that was found.
type Repo struct {
	// Path is the absolute path with symbolic links resolved.
	Path string
	// Name is the path relative to the search root, with / as the separator.
	// A repository found through a symbolic link is named by the link path, not by its target.
	// With several roots it starts with the base name of the root, such as "work/app".
	// When the root itself is a repository, Name is the base name of the root.
	Name string
	// Bare reports whether the repository has no work tree.
	Bare bool
}

// Options controls the search.
// The zero value has no depth limit, uses the default skip list, does not follow symbolic links,
// and drops warnings.
type Options struct {
	// MaxDepth limits the depth below each root, where the root is 0. Zero or less means no limit.
	MaxDepth int
	// SkipDirs lists directory names not to enter. Nil means DefaultSkipDirs.
	SkipDirs []string
	// FollowSymlinks enters symbolic links to directories.
	// A link is not followed when its target is inside the current root,
	// because the walk reaches the target directly.
	// A link is also not followed when its target contains, or is inside, a directory already searched.
	// This stops loops and searches each directory once.
	FollowSymlinks bool
	// Warn is called for directories that cannot be read and are skipped. Nil means no call.
	Warn func(path string, err error)
	// Progress is called for each visited directory.
	// dirs is the number of directories seen so far and repos is the number of repositories found.
	// Nil means no call.
	Progress func(dirs, repos int, path string)
}

func (o Options) warn(path string, err error) {
	if o.Warn != nil {
		o.Warn(path, err)
	}
}

// search holds the state of one Discover call.
type search struct {
	o        Options
	skipDirs []string
	multi    bool     // Whether there are several roots.
	base     string   // The current root with symbolic links resolved.
	entered  []string // Roots and link targets that were searched, with links resolved.
	seen     map[string]bool
	repos    []Repo
	dirs     int
}

// walk searches dir, which has no symbolic links in its path.
// shown is the path of dir as the user sees it, through the links that led there.
// Names, depth, warnings, and progress use shown.
func (s *search) walk(dir, shown string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		shownPath := shown
		if rel, relErr := filepath.Rel(dir, path); relErr == nil && rel != "." {
			shownPath = filepath.Join(shown, rel)
		}
		if err != nil {
			if path == s.base {
				return err
			}
			// One unreadable directory out of ten must not hide all results.
			s.o.warn(shownPath, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if s.o.FollowSymlinks && !slices.Contains(s.skipDirs, d.Name()) {
				return s.follow(path, shownPath)
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		// The skip list is checked on the link name for a link target, so only dir itself is exempt.
		if path != dir && slices.Contains(s.skipDirs, d.Name()) {
			return fs.SkipDir
		}
		s.dirs++
		if s.o.Progress != nil {
			s.o.Progress(s.dirs, len(s.repos), shownPath)
		}
		if bare, ok := detect(path); ok {
			if !s.seen[path] {
				s.seen[path] = true
				name := repoName(s.base, shownPath)
				// With several roots, repositories with the same name (such as "app")
				// could not be told apart. Prefix them with the base name of the root.
				if s.multi && shownPath != s.base {
					name = filepath.Base(s.base) + "/" + name
				}
				s.repos = append(s.repos, Repo{Path: path, Name: name, Bare: bare})
			}
			return fs.SkipDir
		}
		if s.o.MaxDepth > 0 && depth(s.base, shownPath) >= s.o.MaxDepth {
			return fs.SkipDir
		}
		return nil
	})
}

// follow searches the target of the symbolic link at path when the target is a new directory.
func (s *search) follow(path, shownPath string) error {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		// Broken links are common and harmless, so they are not reported.
		if !errors.Is(err, fs.ErrNotExist) {
			s.o.warn(shownPath, err)
		}
		return nil
	}
	if !isDir(target) {
		return nil
	}
	if within(target, s.base) {
		return nil
	}
	for _, dir := range s.entered {
		if within(target, dir) || within(dir, target) {
			return nil
		}
	}
	s.entered = append(s.entered, target)
	return s.walk(target, shownPath)
}

// Discover returns the Git repositories under roots, sorted by path within each root.
//
// The search follows these rules.
//   - A repository is not searched further, so submodules and vendored repositories are not counted twice.
//   - Symbolic links are followed only with Options.FollowSymlinks. A root that is a link is always resolved.
//   - Directories that cannot be read, for example because of permissions, go to Warn and the search continues.
//
// It returns an error only when a root does not exist or is not a directory.
func Discover(roots []string, o Options) ([]Repo, error) {
	s := &search{o: o, skipDirs: o.SkipDirs, multi: len(roots) > 1, seen: map[string]bool{}}
	if s.skipDirs == nil {
		s.skipDirs = DefaultSkipDirs
	}
	for _, root := range roots {
		base, err := resolveRoot(root)
		if err != nil {
			return nil, err
		}
		s.base = base
		s.entered = append(s.entered, base)
		if err := s.walk(base, base); err != nil {
			return nil, fmt.Errorf("%s: %w", root, err)
		}
	}
	return s.repos, nil
}

// resolveRoot makes root absolute and resolves symbolic links.
// WalkDir does not follow links, so a root that is a link could not be entered otherwise.
func resolveRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("%s: %w", root, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("%s: %w", root, err)
	}
	if !isDir(resolved) {
		return "", fmt.Errorf("%s: not a directory", root)
	}
	return resolved, nil
}

// detect reports whether dir is a Git repository.
//
// It recognizes these layouts.
//   - A .git directory is a normal repository.
//   - A .git file that starts with "gitdir: " is a worktree or submodule.
//   - A HEAD file with objects/ and refs/ directories is a bare repository.
func detect(dir string) (bare, ok bool) {
	dotGit := filepath.Join(dir, ".git")
	if info, err := os.Stat(dotGit); err == nil {
		if info.IsDir() || (info.Mode().IsRegular() && isGitdirFile(dotGit)) {
			return false, true
		}
	}
	if isFile(filepath.Join(dir, "HEAD")) &&
		isDir(filepath.Join(dir, "objects")) &&
		isDir(filepath.Join(dir, "refs")) {
		return true, true
	}
	return false, false
}

// isGitdirFile reports whether path is the .git file of a worktree or submodule ("gitdir: <path>").
func isGitdirFile(path string) bool {
	f, err := os.Open(path) //nolint:gosec // A .git file from the walk.
	if err != nil {
		return false
	}
	defer f.Close()

	prefix := []byte("gitdir: ")
	buf := make([]byte, len(prefix))
	if _, err := io.ReadFull(f, buf); err != nil {
		return false
	}
	return bytes.Equal(buf, prefix)
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// repoName returns path relative to base with / as the separator.
// It returns the base name when path is base itself.
func repoName(base, path string) string {
	if path == base {
		return filepath.Base(base)
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// depth returns the depth of path below base, where base is 0.
func depth(base, path string) int {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

// within reports whether path is dir or a path inside dir.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
