package main

// The internal packages test each part.
// The tests in this package check argument handling and whole runs.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsutsu3/git-when/internal/gittest"
)

// TestMain points the default cache directory at a temporary directory.
// This keeps tests from writing to the user's cache (~/.cache/git-when).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "git-when-test-cache-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defaultCacheDir = func() (string, error) { return dir, nil }
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// defaults returns the options for no flags, with mod applied.
func defaults(mod func(*options)) options {
	o := options{
		roots:     []string{"."},
		format:    "term",
		out:       "-",
		maxDepth:  defaultMaxDepth,
		view:      "heatmap",
		weekStart: "mon",
		weekend:   "sat,sun",
		scale:     "sqrt",
		color:     "never",
		theme:     "auto",
		progress:  "auto",
		timings:   "auto",
	}
	if mod != nil {
		mod(&o)
	}
	return o
}

func mustResolve(t *testing.T, args []string) config {
	t.Helper()
	o, err := parseArgs(args, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := resolve(o, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// fixture creates a directory tree for end-to-end tests.
// It has an empty repository, a broken repository, and a repository inside node_modules.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	place(t, gittest.New(t,
		gittest.CommitAt("2024-03-05T09:00:00+09:00"), // Tuesday 09:00
		gittest.CommitAt("2024-03-04T09:30:00-08:00"), // Monday 09:00
	), root, "app")
	place(t, gittest.New(t), root, "empty")
	dep := gittest.New(t, gittest.CommitAt("2024-03-09T15:00:00+09:00"))
	place(t, dep, root, "node_modules/dep")
	// A worktree whose gitdir does not exist.
	// Discovery finds it, but git log fails.
	writeFile(t, root, "broken/.git", "gitdir: "+filepath.Join(root, "missing")+"\n")
	return root
}

func runOK(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := run(t.Context(), args, &out, &errOut); code != exitOK {
		t.Fatalf("run(%q) exit code = %d, stderr:\n%s", args, code, errOut.String())
	}
	return out.String(), errOut.String()
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

// wellFormed fails the test if s is not well-formed XML.
func wellFormed(t *testing.T, name, s string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(s))
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("%s is not well-formed XML: %v", name, err)
		}
	}
}
