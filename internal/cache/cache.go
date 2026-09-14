// Package cache stores slow results on disk and reuses them.
//
// Callers build the key with Key from everything that can change the result.
// The cache only makes runs faster. It never affects correctness.
// A broken or outdated entry is treated as missing and is simply built again.
package cache

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

// magic identifies the entry format at the start of each entry.
// Change it when the stored format changes.
const magic = "git-when-cache/1\n"

// Cache is a cache stored in a directory.
// A nil Cache stores nothing. Get always misses and Put does nothing.
type Cache struct {
	dir string
}

// New returns a Cache stored in dir. It returns nil when dir is empty.
func New(dir string) *Cache {
	if dir == "" {
		return nil
	}
	return &Cache{dir: dir}
}

// Get returns the data stored for key.
// ok is false when the entry is missing, broken, or in another format.
func (c *Cache) Get(key string) (data []byte, ok bool) {
	if c == nil {
		return nil, false
	}
	raw, err := os.ReadFile(c.path(key))
	if err != nil || !bytes.HasPrefix(raw, []byte(magic)) {
		return nil, false
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw[len(magic):]))
	if err != nil {
		return nil, false
	}
	// Reading to the end checks the trailing checksum, so a truncated entry fails here.
	data, err = io.ReadAll(zr)
	if err != nil {
		return nil, false
	}
	return data, true
}

// Put stores data for key.
func (c *Cache) Put(key string, data []byte) error {
	if c == nil {
		return nil
	}
	if err := os.MkdirAll(c.dir, 0o750); err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.WriteString(magic)
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	// Write to a temporary file and rename it.
	// A crash during the write then leaves no broken entry.
	tmp, err := os.CreateTemp(c.dir, key+".*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // After the rename the file is gone, so this does nothing.
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), c.path(key))
}

func (c *Cache) path(key string) string {
	return filepath.Join(c.dir, filepath.Base(key))
}

// Key returns a key built from parts.
// The length of each part is mixed in, so ("ab", "c") and ("a", "bc") give different keys.
func Key(parts ...string) string {
	h := sha256.New()
	var size [8]byte
	for _, p := range parts {
		binary.BigEndian.PutUint64(size[:], uint64(len(p)))
		h.Write(size[:])
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}
