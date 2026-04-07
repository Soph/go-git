package compat

import (
	"errors"
	"io"
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

func TestFileMappingAddSyncsAndReusesAppendHandle(t *testing.T) {
	base := memfs.New()
	_ = base.MkdirAll("objects", 0755)

	fs := &trackingOpenFileFS{Filesystem: base}
	m := NewFileMapping(fs, "objects")

	native1 := plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	compat1 := plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	native2 := plumbing.NewHash("cccccccccccccccccccccccccccccccccccccccc")
	compat2 := plumbing.NewHash("dddddddddddddddddddddddddddddddddddddddd")

	require.NoError(t, m.Add(native1, compat1))
	require.NoError(t, m.Add(native2, compat2))

	assert.Equal(t, 1, fs.openCount)
	require.NotNil(t, fs.lastFile)
	assert.Equal(t, 2, fs.lastFile.syncCount)
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

type trackingOpenFileFS struct {
	billy.Filesystem
	openCount int
	lastFile  *syncTrackingFile
}

func (fs *trackingOpenFileFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	f, err := fs.Filesystem.OpenFile(filename, flag, perm)
	if err != nil {
		return nil, err
	}
	if flag&(os.O_APPEND|os.O_WRONLY) == (os.O_APPEND | os.O_WRONLY) {
		fs.openCount++
		fs.lastFile = &syncTrackingFile{File: f}
		return fs.lastFile, nil
	}

	return f, nil
}

type syncTrackingFile struct {
	billy.File
	syncCount int
}

func (f *syncTrackingFile) Sync() error {
	f.syncCount++
	if syncer, ok := f.File.(billy.Syncer); ok {
		return syncer.Sync()
	}
	if seeker, ok := f.File.(io.Seeker); ok {
		_, err := seeker.Seek(0, io.SeekCurrent)
		return err
	}
	return nil
}
