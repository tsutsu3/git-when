package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPutGet(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cache") // Created if missing.
	c := New(dir)
	key := Key("repo", "HEAD")
	data := bytes.Repeat([]byte("2024-03-05T09:00:00+09:00\tAlice\talice@example.com\n"), 1000)

	if _, ok := c.Get(key); ok {
		t.Fatal("Get before Put hit")
	}
	if err := c.Put(key, data); err != nil {
		t.Fatal(err)
	}
	got, ok := c.Get(key)
	if !ok || !bytes.Equal(got, data) {
		t.Fatalf("Get = %d bytes, %v; want the %d bytes put", len(got), ok, len(data))
	}

	// Entries can be overwritten and no temporary file is left.
	if err := c.Put(key, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if got, ok := c.Get(key); !ok || string(got) != "new" {
		t.Errorf("after overwrite Get = %q, %v", got, ok)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != key {
		t.Errorf("cache dir has %v, want only %s", entries, key)
	}
	// Repetitive git log output is compressed.
	info, err := os.Stat(filepath.Join(dir, key))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() >= int64(len(data))/10 {
		t.Errorf(
			"entry is %d bytes for %d bytes of data; want it compressed",
			info.Size(),
			len(data),
		)
	}
}

func TestGetIgnoresBrokenEntries(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)
	key := Key("repo")
	if err := c.Put(key, []byte("some git log output")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, key)
	good, err := os.ReadFile(path) //nolint:gosec // A file in a test temp dir.
	if err != nil {
		t.Fatal(err)
	}

	for name, content := range map[string][]byte{
		"not a cache file":  []byte("hello"),
		"other version":     append([]byte("git-when-cache/0\n"), good[len(magic):]...),
		"truncated":         good[:len(good)-4], // The gzip checksum is missing.
		"magic but no gzip": []byte(magic + "plain text"),
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
			if got, ok := c.Get(key); ok {
				t.Errorf("Get = %q, true; want a miss", got)
			}
		})
	}
}

func TestNilCache(t *testing.T) {
	c := New("")
	if c != nil {
		t.Fatalf("New(\"\") = %v, want nil", c)
	}
	if err := c.Put("k", []byte("v")); err != nil {
		t.Errorf("nil Put = %v", err)
	}
	if _, ok := c.Get("k"); ok {
		t.Error("nil Get hit")
	}
}

func TestKey(t *testing.T) {
	if first, second := Key("a", "b"), Key("a", "b"); first != second {
		t.Error("Key is not deterministic")
	}
	for _, pair := range [][2][]string{
		{{"ab", "c"}, {"a", "bc"}},
		{{"a", ""}, {"a"}},
		{{"a", "b"}, {"b", "a"}},
	} {
		if Key(pair[0]...) == Key(pair[1]...) {
			t.Errorf("Key(%q) == Key(%q)", pair[0], pair[1])
		}
	}
	if len(Key()) != 64 {
		t.Errorf("Key() = %q, want 64 hex digits", Key())
	}
}
