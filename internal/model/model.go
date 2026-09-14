// Package model defines the core data model of git-when.
//
// Commits are counted once over six dimensions.
// They are author, project, year, month, weekday, and hour.
// Every view reads the axes it needs from this Aggregate, so history is never scanned again.
package model

import (
	"fmt"
	"slices"
	"time"
)

const (
	// MaxYear is the largest year that fits in Key (12 bits), which is 4095.
	MaxYear = 1<<12 - 1
	// maxAuthors is the number of authors that fit in Key.Author (16 bits).
	maxAuthors = 1 << 16
)

// isoWeekday maps time.Weekday (Sunday=0) to a Monday=0 weekday number.
var isoWeekday = [7]uint8{6, 0, 1, 2, 3, 4, 5}

// Author is one commit author, after .mailmap is applied.
type Author struct {
	// Name is the author name.
	Name string
	// Email is the author email.
	Email string
}

// Project is one analyzed repository.
type Project struct {
	// Name is the display name, such as the path relative to the search root.
	Name string
	// Path is the absolute path.
	Path string
	// Head is HEAD at analysis time. It is recorded for reproducibility.
	Head string
	// Commits is the number of commits counted.
	Commits int
	// First is the time of the first commit. git-when does not set it yet.
	First time.Time
	// Last is the time of the last commit. git-when does not set it yet.
	Last time.Time
}

// Key is the position of a bucket in the six dimensions.
// Weekday always uses 0=Monday..6=Sunday.
// --week-start changes only the display order, not the stored value.
type Key struct {
	// Author is an index into Aggregate.Authors.
	Author uint16
	// Project is an index into Aggregate.Projects.
	Project uint16
	// Year is the calendar year, from 0 to MaxYear.
	Year uint16
	// Month is 0-11.
	Month uint8
	// Weekday is 0=Mon..6=Sun.
	Weekday uint8
	// Hour is 0-23.
	Hour uint8
}

// Bit layout: author[40:56] project[24:40] year[12:24] month[8:12] weekday[5:8] hour[0:5]
//
// Larger fields sit in higher bits.
// Sorting packed values therefore sorts by author, project, year, month, weekday, and hour.
const (
	shHour    = 0
	shWeekday = 5
	shMonth   = 8
	shYear    = 12
	shProject = 24
	shAuthor  = 40
)

// Unpack is the inverse of Pack.
func Unpack(u uint64) Key {
	return Key{
		Author:  uint16(u >> shAuthor & 0xFFFF),
		Project: uint16(u >> shProject & 0xFFFF),
		Year:    uint16(u >> shYear & 0xFFF),
		Month:   uint8(u >> shMonth & 0xF),
		Weekday: uint8(u >> shWeekday & 0x7),
		Hour:    uint8(u >> shHour & 0x1F),
	}
}

// Pack packs k into an integer for use as a map key.
// The caller (Add) makes sure every field is in range.
func (k Key) Pack() uint64 {
	return uint64(k.Author)<<shAuthor |
		uint64(k.Project)<<shProject |
		uint64(k.Year)<<shYear |
		uint64(k.Month)<<shMonth |
		uint64(k.Weekday)<<shWeekday |
		uint64(k.Hour)<<shHour
}

// OffsetKey counts commits per recorded UTC offset for the tzshift view.
type OffsetKey struct {
	// Author is an index into Aggregate.Authors.
	Author uint16
	// Project is an index into Aggregate.Projects.
	Project uint16
	// Year is the calendar year.
	Year uint16
	// Offset is the UTC offset in seconds.
	Offset int32
}

// Filters records the filters that were applied, so the output can be reproduced.
type Filters struct {
	// Author is the --author regular expression.
	Author string `json:"author,omitempty"`
	// ExcludeAuthor is the --exclude-author regular expression.
	ExcludeAuthor string `json:"excludeAuthor,omitempty"`
	// Since is reserved for a start date filter. git-when does not set it yet.
	Since string `json:"since,omitempty"`
	// Until is reserved for an end date filter. git-when does not set it yet.
	Until string `json:"until,omitempty"`
	// Merges reports whether merge commits were counted. git-when always skips merges.
	Merges bool `json:"merges"`
	// Bots reports whether bot commits were counted.
	Bots bool `json:"bots"`
	// Rev is the revision that was read. It is always "HEAD".
	Rev string `json:"rev"`
}

// Meta records how the output was made. Every output format embeds it.
// Commit hashes and the command line make the result reproducible.
type Meta struct {
	// Tool is always "git-when".
	Tool string `json:"tool"`
	// Version is the git-when version.
	Version string `json:"version"`
	// TZ names the time basis. It is always "author" (author local time) and stays for schema 1.
	TZ string `json:"tz"`
	// Period is the first and last month, such as "2023-01 to 2026-09", or "-" without commits.
	Period string `json:"period"`
	// WeekStart is the first day of the week, 0=Mon..6=Sun.
	WeekStart int `json:"weekStart"`
	// Weekend lists the weekend days, 0=Mon..6=Sun.
	Weekend []int `json:"weekend"`
	// MinTotal is --min-total. The HTML report hides authors with fewer commits from its author list.
	// It is omitted when zero, so the JSON of runs without the flag does not change.
	MinTotal int `json:"minTotal,omitempty"`
	// DefaultAuthor is the index in Authors that the HTML report selects when it opens.
	// It comes from --default-author. Nil means all authors, and the field is then omitted.
	DefaultAuthor *int `json:"defaultAuthor,omitempty"`
	// NoGitHubLink hides the link to the git-when repository in the HTML report.
	// It comes from --no-github-link and is omitted when false.
	NoGitHubLink bool `json:"noGitHubLink,omitempty"`
	// NoEmail leaves author emails out of the HTML report. It comes from --no-email.
	// Authors keep their indexes, and the page numbers authors that share a name.
	// The field is omitted when false.
	NoEmail bool `json:"noEmail,omitempty"`
	// Command is the command line, quoted for a shell.
	Command string `json:"command"`
	// Roots lists the directories that were searched.
	Roots []string `json:"roots"`
	// Filters records the applied filters.
	Filters Filters `json:"filters"`
}

// Aggregate is the only input of every view.
type Aggregate struct {
	// Authors lists authors in order of first appearance. Keys refer to them by index.
	Authors []Author
	// Projects lists repositories in discovery order. Keys refer to them by index.
	Projects []Project
	// Buckets maps Key.Pack() to a commit count.
	Buckets map[uint64]uint32
	// Offsets maps an OffsetKey to a commit count.
	Offsets map[OffsetKey]uint32
	// Meta records how the aggregate was made.
	Meta Meta

	authorIdx map[Author]uint16 // Built lazily from Authors for AuthorIndex.
}

// New returns an empty Aggregate.
func New() *Aggregate {
	return &Aggregate{
		Buckets: make(map[uint64]uint32),
		Offsets: make(map[OffsetKey]uint32),
	}
}

// AuthorIndex returns the index of au in Authors. A new author is appended.
// Authors are the same person only when both name and email match.
// Merging identities is left to .mailmap.
func (a *Aggregate) AuthorIndex(au Author) (uint16, error) {
	if a.authorIdx == nil {
		// Build the index here so it also matches an Aggregate whose Authors were set directly.
		a.authorIdx = make(map[Author]uint16, len(a.Authors))
		for i, known := range a.Authors {
			a.authorIdx[known] = uint16(i) //nolint:gosec // Checked below.
		}
	}
	if i, ok := a.authorIdx[au]; ok {
		return i, nil
	}
	if len(a.Authors) >= maxAuthors {
		return 0, fmt.Errorf("too many authors: more than %d", maxAuthors)
	}
	i := uint16(len(a.Authors)) //nolint:gosec // The check above keeps this under maxAuthors.
	a.Authors = append(a.Authors, au)
	a.authorIdx[au] = i
	return i, nil
}

// Add adds one commit to the aggregate.
// t is the author date in the author's own UTC offset, so its clock fields are author local time.
// offsetSec is that recorded UTC offset in seconds.
func (a *Aggregate) Add(author, project uint16, t time.Time, offsetSec int32) error {
	y := t.Year()
	if y < 0 || y > MaxYear {
		return fmt.Errorf("year %d out of representable range 0-%d", y, MaxYear)
	}
	year := uint16(y)

	k := Key{
		Author:  author,
		Project: project,
		Year:    year,
		Month:   uint8(t.Month() - 1), //nolint:gosec // time.Month is 1-12.
		Weekday: ISOWeekday(t.Weekday()),
		Hour:    uint8(t.Hour()), //nolint:gosec // time.Hour is 0-23.
	}
	a.Buckets[k.Pack()]++
	a.Offsets[OffsetKey{Author: author, Project: project, Year: year, Offset: offsetSec}]++
	return nil
}

// Total returns the sum of all buckets.
func (a *Aggregate) Total() int {
	n := 0
	for _, c := range a.Buckets {
		n += int(c)
	}
	return n
}

// Years returns the years that appear in Buckets and Offsets, in ascending order.
func (a *Aggregate) Years() []int {
	seen := map[int]struct{}{}
	for u := range a.Buckets {
		seen[int(Unpack(u).Year)] = struct{}{}
	}
	for k := range a.Offsets {
		seen[int(k.Year)] = struct{}{}
	}
	out := make([]int, 0, len(seen))
	for y := range seen {
		out = append(out, y)
	}
	slices.Sort(out)
	return out
}

// SortedKeys returns the keys of Buckets in ascending order for deterministic output.
func (a *Aggregate) SortedKeys() []uint64 {
	keys := make([]uint64, 0, len(a.Buckets))
	for u := range a.Buckets {
		keys = append(keys, u)
	}
	slices.Sort(keys)
	return keys
}

// SortedOffsetKeys returns the keys of Offsets for deterministic output.
// They are sorted by author, project, year, and offset.
func (a *Aggregate) SortedOffsetKeys() []OffsetKey {
	keys := make([]OffsetKey, 0, len(a.Offsets))
	for k := range a.Offsets {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(x, y OffsetKey) int {
		switch {
		case x.Author != y.Author:
			return int(x.Author) - int(y.Author)
		case x.Project != y.Project:
			return int(x.Project) - int(y.Project)
		case x.Year != y.Year:
			return int(x.Year) - int(y.Year)
		default:
			return int(x.Offset) - int(y.Offset)
		}
	})
	return keys
}

// ISOWeekday converts time.Weekday (Sunday=0) to Monday=0.
func ISOWeekday(w time.Weekday) uint8 {
	return isoWeekday[w]
}
