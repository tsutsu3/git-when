// Package gittest creates Git repositories with known commit dates for tests.
//
// The repositories do not depend on the local Git settings
// (~/.gitconfig, /etc/gitconfig, or GIT_* environment variables).
package gittest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Identity used for commits without an explicit author.
const (
	DefaultName  = "Alice"
	DefaultEmail = "alice@example.com"
)

// Commit describes one commit to create.
type Commit struct {
	// AuthorDate is the author date in RFC 3339, such as "2024-03-05T09:00:00+09:00".
	AuthorDate string
	// CommitterDate is the committer date. Empty means AuthorDate.
	CommitterDate string
	// Name and Email identify the author and committer.
	// Empty values mean DefaultName and DefaultEmail.
	Name  string
	Email string
	// Message is the commit message. Empty means a numbered message.
	Message string
}

// CommitAt returns a commit by the default author at date.
func CommitAt(date string) Commit {
	return Commit{AuthorDate: date}
}

// CommitBy returns a commit by name <email> at date.
func CommitBy(name, email, date string) Commit {
	return Commit{AuthorDate: date, Name: name, Email: email}
}

// New creates a non-bare repository with commits in order and returns its path.
// With no commits, the repository has no commits at all.
func New(t testing.TB, commits ...Commit) string {
	t.Helper()

	dir := t.TempDir()
	Run(t, dir, nil, "init", "--quiet", "--initial-branch=main")
	for i, c := range commits {
		commit(t, dir, i, c)
	}
	return dir
}

// NewBare creates a bare repository with commits and returns its path.
// With no commits, the bare repository has no commits at all.
func NewBare(t testing.TB, commits ...Commit) string {
	t.Helper()

	dst := filepath.Join(t.TempDir(), "repo.git")
	if len(commits) == 0 {
		Run(t, ".", nil, "init", "--quiet", "--bare", "--initial-branch=main", dst)
		return dst
	}
	src := New(t, commits...)
	Run(t, ".", nil, "clone", "--quiet", "--bare", src, dst)
	return dst
}

// Run runs git in dir and returns its standard output. It stops the test on failure.
// extraEnv is added after the isolated environment.
func Run(t testing.TB, dir string, extraEnv []string, args ...string) string {
	t.Helper()

	//nolint:gosec // Only arguments built by test code are passed.
	var stderr bytes.Buffer
	cmd := exec.CommandContext( //nolint:gosec // Only arguments built by test code are passed.
		t.Context(),
		"git",
		append([]string{"-C", dir}, args...)...)
	cmd.Env = append(Env(), extraEnv...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return string(out)
}

// Env returns environment variables for tests that are isolated from Git settings.
func Env() []string {
	return []string{
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"TZ=UTC",
		"LC_ALL=C",
	}
}

func commit(t testing.TB, dir string, i int, c Commit) {
	t.Helper()

	authorDate := gitDate(t, c.AuthorDate)
	committerDate := authorDate
	if c.CommitterDate != "" {
		committerDate = gitDate(t, c.CommitterDate)
	}

	name, email := c.Name, c.Email
	if name == "" {
		name = DefaultName
	}
	if email == "" {
		email = DefaultEmail
	}

	msg := c.Message
	if msg == "" {
		msg = fmt.Sprintf("commit %d", i)
	}

	Run(t, dir, []string{
		"GIT_AUTHOR_NAME=" + name,
		"GIT_AUTHOR_EMAIL=" + email,
		"GIT_AUTHOR_DATE=" + authorDate,
		"GIT_COMMITTER_NAME=" + name,
		"GIT_COMMITTER_EMAIL=" + email,
		"GIT_COMMITTER_DATE=" + committerDate,
	}, "commit", "--quiet", "--allow-empty", "--no-verify", "-m", msg)
}

// gitDate converts an RFC 3339 date to Git's internal form "@<unix> <±hhmm>".
// This avoids differences in how Git parses dates and passes the time and offset exactly.
func gitDate(t testing.TB, s string) string {
	t.Helper()

	d, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("gittest: invalid date %q: %v", s, err)
	}
	return fmt.Sprintf("@%d %s", d.Unix(), d.Format("-0700"))
}
