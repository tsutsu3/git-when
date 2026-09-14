package model

import (
	"encoding/json"
	"time"
)

// The JSON form of an Aggregate (schema 1).
//
// A map[uint64]uint32 cannot be written as JSON directly.
// The keys are sorted and written as flat arrays.
//
//	buckets: [author, project, yearIdx, month, weekday, hour, count, ...]
//	offsets: [author, project, yearIdx, offsetSec, count, ...]
//
// yearIdx is an index into the years array.

// Schema is the version of the JSON format. Increase it when the shape changes.
const Schema = 1

// Number of array elements per record in the flat arrays.
const (
	// BucketStride is the number of elements per bucket record.
	BucketStride = 7
	// OffsetStride is the number of elements per offset record.
	OffsetStride = 5
)

type jsonAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type jsonProject struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Head    string    `json:"head"`
	Commits int       `json:"commits"`
	First   time.Time `json:"first,omitzero"`
	Last    time.Time `json:"last,omitzero"`
}

type jsonDoc struct {
	Schema   int           `json:"schema"`
	Meta     Meta          `json:"meta"`
	Authors  []jsonAuthor  `json:"authors"`
	Projects []jsonProject `json:"projects"`
	Years    []int         `json:"years"`
	Buckets  []int64       `json:"buckets"`
	Offsets  []int64       `json:"offsets"`
}

// MarshalJSON writes the Aggregate as deterministic JSON.
func (a *Aggregate) MarshalJSON() ([]byte, error) {
	years := a.Years()

	authors := make([]jsonAuthor, len(a.Authors))
	for i, au := range a.Authors {
		authors[i] = jsonAuthor(au)
	}
	projects := make([]jsonProject, len(a.Projects))
	for i, p := range a.Projects {
		projects[i] = jsonProject(p)
	}

	yearIdx := make(map[int]int64, len(years))
	for i, y := range years {
		yearIdx[y] = int64(i)
	}

	keys := a.SortedKeys()
	buckets := make([]int64, 0, len(keys)*BucketStride)
	for _, u := range keys {
		k := Unpack(u)
		buckets = append(buckets,
			int64(k.Author), int64(k.Project), yearIdx[int(k.Year)],
			int64(k.Month), int64(k.Weekday), int64(k.Hour), int64(a.Buckets[u]))
	}

	oks := a.SortedOffsetKeys()
	offsets := make([]int64, 0, len(oks)*OffsetStride)
	for _, k := range oks {
		offsets = append(offsets,
			int64(k.Author), int64(k.Project), yearIdx[int(k.Year)],
			int64(k.Offset), int64(a.Offsets[k]))
	}

	return json.Marshal(jsonDoc{
		Schema:   Schema,
		Meta:     a.Meta,
		Authors:  authors,
		Projects: projects,
		Years:    years,
		Buckets:  buckets,
		Offsets:  offsets,
	})
}
