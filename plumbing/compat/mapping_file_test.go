package compat

import (
	"testing"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileMapping(t *testing.T) {
	testHashMapping(t, func() HashMapping {
		fs := memfs.New()
		_ = fs.MkdirAll("objects", 0755)
		return NewFileMapping(fs, "objects")
	})
}

func TestFileMappingPersistence(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("objects", 0755)

	native := plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	compat := plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	// Write a mapping with one instance.
	m1 := NewFileMapping(fs, "objects")
	require.NoError(t, m1.Add(native, compat))

	// Open a fresh instance and verify it can read the persisted mapping.
	m2 := NewFileMapping(fs, "objects")
	got, err := m2.NativeToCompat(native)
	require.NoError(t, err)
	assert.True(t, got.Equal(compat))
}

func TestFileMappingEmptyFile(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("objects", 0755)

	m := NewFileMapping(fs, "objects")
	assert.Equal(t, 0, m.Count())
}
