package model

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
	"time"
)

func TestPackUnpackRoundTrip(t *testing.T) {
	cases := []Key{
		{},
		{Author: 1, Project: 2, Year: 2024, Month: 2, Weekday: 1, Hour: 9},
		// Boundary values of each field.
		{Hour: 23},
		{Weekday: 6},
		{Month: 11},
		{Year: MaxYear},
		{Project: 0xFFFF},
		{Author: 0xFFFF},
		{Author: 0xFFFF, Project: 0xFFFF, Year: MaxYear, Month: 11, Weekday: 6, Hour: 23},
	}
	for _, k := range cases {
		if got := Unpack(k.Pack()); got != k {
			t.Errorf("Unpack(Pack(%+v)) = %+v", k, got)
		}
	}
}

func TestPackFieldsDoNotOverlap(t *testing.T) {
	// When only one field is at its maximum, the other fields come back as zero.
	fields := []Key{
		{Hour: 0x1F},
		{Weekday: 0x7},
		{Month: 0xF},
		{Year: 0xFFF},
		{Project: 0xFFFF},
		{Author: 0xFFFF},
	}
	var union uint64
	for _, k := range fields {
		p := k.Pack()
		if union&p != 0 {
			t.Errorf("%+v overlaps bits %#x", k, union&p)
		}
		union |= p
	}
}

// TestPackOrder checks that packed integers sort by author, project, year,
// month, weekday, and hour, in that order.
func TestPackOrder(t *testing.T) {
	cases := []struct {
		name       string
		small, big Key
	}{
		{
			"author beats everything below",
			Key{Author: 0, Project: 0xFFFF, Year: MaxYear, Month: 11, Weekday: 6, Hour: 23},
			Key{Author: 1},
		},
		{
			"project beats year and below",
			Key{Project: 0, Year: MaxYear, Month: 11, Weekday: 6, Hour: 23},
			Key{Project: 1},
		},
		{
			"year beats month and below",
			Key{Year: 2023, Month: 11, Weekday: 6, Hour: 23},
			Key{Year: 2024},
		},
		{
			"month beats weekday and below",
			Key{Month: 0, Weekday: 6, Hour: 23},
			Key{Month: 1},
		},
		{
			"weekday beats hour",
			Key{Weekday: 0, Hour: 23},
			Key{Weekday: 1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.small.Pack() >= c.big.Pack() {
				t.Errorf("Pack(%+v) = %#x >= Pack(%+v) = %#x",
					c.small, c.small.Pack(), c.big, c.big.Pack())
			}
		})
	}
}

func TestAuthorIndex(t *testing.T) {
	alice := Author{"Alice", "alice@example.com"}
	bob := Author{"Bob", "bob@example.com"}

	a := New()
	for _, c := range []struct {
		au   Author
		want uint16
	}{
		{alice, 0},
		{bob, 1},
		{alice, 0}, // The second call returns the same index.
		// The same name with another email is another person. Merging is left to .mailmap.
		{Author{"Alice", "alice@home.example"}, 2},
	} {
		got, err := a.AuthorIndex(c.au)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("AuthorIndex(%+v) = %d, want %d", c.au, got, c.want)
		}
	}
	if len(a.Authors) != 3 || a.Authors[0] != alice || a.Authors[1] != bob {
		t.Errorf("Authors = %+v", a.Authors)
	}

	// An aggregate with Authors set directly also returns existing indexes.
	b := &Aggregate{Authors: []Author{bob, alice}}
	if got, err := b.AuthorIndex(alice); err != nil || got != 1 {
		t.Errorf("AuthorIndex on preset Authors = %d, %v; want 1", got, err)
	}
}

func TestAuthorIndexLimit(t *testing.T) {
	a := New()
	for i := range maxAuthors {
		// Start at U+10000 to avoid surrogates (U+D800-DFFF) and make every author unique.
		email := string(rune(0x10000+i)) + "@example.com"
		if _, err := a.AuthorIndex(Author{Email: email}); err != nil {
			t.Fatalf("author #%d: %v", i, err)
		}
	}
	if _, err := a.AuthorIndex(Author{Email: "one-too-many@example.com"}); err == nil {
		t.Errorf("AuthorIndex beyond %d authors succeeded, want error", maxAuthors)
	}
}

func TestAdd(t *testing.T) {
	jst := time.FixedZone("JST", 9*3600)
	a := New()
	// 2024-03-05 is a Tuesday.
	tue := time.Date(2024, time.March, 5, 9, 15, 0, 0, jst)
	for range 2 {
		if err := a.Add(1, 2, tue, 9*3600); err != nil {
			t.Fatal(err)
		}
	}
	// The same time by another author is another bucket.
	if err := a.Add(0, 2, tue, 9*3600); err != nil {
		t.Fatal(err)
	}

	// check author1
	k := Key{Author: 1, Project: 2, Year: 2024, Month: 2, Weekday: 1, Hour: 9}
	if got := a.Buckets[k.Pack()]; got != 2 {
		t.Errorf("Buckets[%+v] = %d, want 2", k, got)
	}
	ok := OffsetKey{Author: 1, Project: 2, Year: 2024, Offset: 9 * 3600}
	if got := a.Offsets[ok]; got != 2 {
		t.Errorf("Offsets[%+v] = %d, want 2", ok, got)
	}
	// check author0 + author1 total
	if got := a.Total(); got != 3 {
		t.Errorf("Total = %d, want 3", got)
	}
}

func TestAddRejectsUnrepresentableYear(t *testing.T) {
	// Years below 0 or above MaxYear are errors.
	for _, y := range []int{-1, MaxYear + 1} {
		a := New()
		if err := a.Add(0, 0, time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC), 0); err == nil {
			t.Errorf("Add(year=%d) succeeded, want error", y)
		}
		if len(a.Buckets) != 0 || len(a.Offsets) != 0 {
			t.Errorf("Add(year=%d) modified the aggregate", y)
		}
	}

	// MaxYear itself is allowed.
	a := New()
	if err := a.Add(
		0,
		0,
		time.Date(MaxYear, time.December, 31, 23, 0, 0, 0, time.UTC),
		0,
	); err != nil {
		t.Errorf("Add(year=%d): %v", MaxYear, err)
	}
}

func TestISOWeekday(t *testing.T) {
	want := map[time.Weekday]uint8{
		time.Monday: 0, time.Tuesday: 1, time.Wednesday: 2, time.Thursday: 3,
		time.Friday: 4, time.Saturday: 5, time.Sunday: 6,
	}
	for w, iso := range want {
		if got := ISOWeekday(w); got != iso {
			t.Errorf("ISOWeekday(%s) = %d, want %d", w, got, iso)
		}
	}
}

func TestMarshalJSONDeterministic(t *testing.T) {
	a := sampleAggregate(t)

	// Map order changes on every run. A missing sort shows up in one of the 20 runs.
	first, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		got, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, first) {
			t.Fatalf("Marshal #%d differs\n got: %s\nwant: %s", i, got, first)
		}
	}
}

func TestMarshalJSONShape(t *testing.T) {
	a := New()
	a.Authors = []Author{{"Alice", "alice@example.com"}, {"Bob", "bob@example.com"}}
	a.Projects = []Project{{Name: "p"}}
	pst := time.FixedZone("PST", -8*3600)
	jst := time.FixedZone("JST", 9*3600)
	mustAdd(t, a, 1, 0, time.Date(2025, time.January, 6, 21, 0, 0, 0, pst), -8*3600) // Monday
	mustAdd(t, a, 0, 0, time.Date(2024, time.March, 5, 9, 0, 0, 0, jst), 9*3600)     // Tuesday
	mustAdd(t, a, 0, 0, time.Date(2024, time.March, 5, 9, 30, 0, 0, jst), 9*3600)

	var doc struct {
		Schema  int     `json:"schema"`
		Years   []int   `json:"years"`
		Buckets []int64 `json:"buckets"`
		Offsets []int64 `json:"offsets"`
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}

	if doc.Schema != Schema {
		t.Errorf("schema = %d, want %d", doc.Schema, Schema)
	}
	if want := []int{2024, 2025}; !slices.Equal(doc.Years, want) {
		t.Errorf("years = %v, want %v", doc.Years, want)
	}
	wantBuckets := []int64{
		// author, project, yearIdx, month, weekday, hour, count
		0, 0, 0, 2, 1, 9, 2,
		1, 0, 1, 0, 0, 21, 1,
	}
	if !slices.Equal(doc.Buckets, wantBuckets) {
		t.Errorf("buckets = %v, want %v", doc.Buckets, wantBuckets)
	}
	wantOffsets := []int64{
		// author, project, yearIdx, offset, count
		0, 0, 0, 9 * 3600, 2,
		1, 0, 1, -8 * 3600, 1,
	}
	if !slices.Equal(doc.Offsets, wantOffsets) {
		t.Errorf("offsets = %v, want %v", doc.Offsets, wantOffsets)
	}
}

func TestMarshalJSONEmpty(t *testing.T) {
	b, err := json.Marshal(New())
	if err != nil {
		t.Fatal(err)
	}
	// Empty arrays are [] and never null. The HTML template needs no special case.
	for _, field := range []string{`"authors":[]`, `"projects":[]`, `"years":[]`, `"buckets":[]`, `"offsets":[]`} {
		if !bytes.Contains(b, []byte(field)) {
			t.Errorf("empty aggregate JSON lacks %s: %s", field, b)
		}
	}
}

// sampleAggregate returns an aggregate with enough buckets to expose random map order.
func sampleAggregate(t *testing.T) *Aggregate {
	t.Helper()
	a := New()
	a.Authors = []Author{{"Alice", "alice@example.com"}, {"Bob", "bob@example.com"}}
	a.Projects = []Project{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	offsets := []int32{9 * 3600, -8 * 3600, 5*3600 + 45*60}
	base := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	for i := range 500 {
		off := offsets[i%len(offsets)]
		ts := base.Add(time.Duration(i*29) * time.Hour).In(time.FixedZone("", int(off)))
		mustAdd(t, a, uint16(i%2), uint16(i%3), ts, off)
	}
	return a
}

func mustAdd(t *testing.T, a *Aggregate, author, project uint16, ts time.Time, off int32) {
	t.Helper()
	if err := a.Add(author, project, ts, off); err != nil {
		t.Fatal(err)
	}
}
