package compat

import (
	"errors"
	"os"
	"testing"

	"github.com/go-git/go-billy/v6"
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

func TestFileMappingAddDoesNotUpdateMemoryOnWriteFailure(t *testing.T) {
	fs := &failingOpenFileFS{
		Filesystem: memfs.New(),
		failPath:   "objects/" + looseObjectIdxFile,
		err:        errors.New("append failed"),
	}
	_ = fs.MkdirAll("objects", 0755)

	native := plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	compat := plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	m := NewFileMapping(fs, "objects")
	err := m.Add(native, compat)
	require.Error(t, err)

	_, err = m.NativeToCompat(native)
	assert.ErrorIs(t, err, plumbing.ErrObjectNotFound)
	assert.Equal(t, 0, m.Count())
}

type failingOpenFileFS struct {
	billy.Filesystem
	failPath string
	err      error
}

func (fs *failingOpenFileFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	if filename == fs.failPath && flag&(os.O_APPEND|os.O_WRONLY) == (os.O_APPEND|os.O_WRONLY) {
		return nil, fs.err
	}

	return fs.Filesystem.OpenFile(filename, flag, perm)
}
