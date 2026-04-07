package compat

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/osfs"
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

func TestFileMappingPersistenceResolvesLatestMapping(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("objects", 0755)

	native1 := plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	compat1 := plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	native2 := plumbing.NewHash("cccccccccccccccccccccccccccccccccccccccc")
	compat2 := plumbing.NewHash("dddddddddddddddddddddddddddddddddddddddd")

	m1 := NewFileMapping(fs, "objects")
	require.NoError(t, m1.Add(native1, compat1))
	require.NoError(t, m1.Add(native2, compat1))
	require.NoError(t, m1.Add(native2, compat2))

	m2 := NewFileMapping(fs, "objects")

	_, err := m2.NativeToCompat(native1)
	assert.ErrorIs(t, err, plumbing.ErrObjectNotFound)

	gotCompat, err := m2.NativeToCompat(native2)
	require.NoError(t, err)
	assert.True(t, gotCompat.Equal(compat2))

	gotNative, err := m2.CompatToNative(compat2)
	require.NoError(t, err)
	assert.True(t, gotNative.Equal(native2))

	_, err = m2.CompatToNative(compat1)
	assert.ErrorIs(t, err, plumbing.ErrObjectNotFound)
}

func TestFileMappingKeepsBoundedCache(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("objects", 0755)

	m := NewFileMapping(fs, "objects")
	for i := 0; i < fileMappingCacheSize*2; i++ {
		native := plumbing.NewHash(fmt.Sprintf("%040x", i+1))
		compat := plumbing.NewHash(fmt.Sprintf("%040x", i+fileMappingCacheSize+1))
		require.NoError(t, m.Add(native, compat))
	}

	assert.LessOrEqual(t, len(m.nativeToCompat.entries), fileMappingCacheSize)
	assert.LessOrEqual(t, len(m.compatToNative.entries), fileMappingCacheSize)
	assert.Equal(t, fileMappingCacheSize*2, m.Count())
}

func TestFileMappingSkipsMalformedLines(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("objects", 0755)

	f, err := fs.Create("objects/" + looseObjectIdxFile)
	require.NoError(t, err)
	_, err = io.WriteString(f, "not-a-mapping\n")
	require.NoError(t, err)
	_, err = io.WriteString(f, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n")
	require.NoError(t, err)
	_, err = io.WriteString(f, "zzzz zzzz\n")
	require.NoError(t, err)
	_, err = io.WriteString(f, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	m := NewFileMapping(fs, "objects")
	got, err := m.NativeToCompat(plumbing.NewHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	require.NoError(t, err)
	assert.True(t, got.Equal(plumbing.NewHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")))
	assert.Equal(t, 1, m.Count())
}

func TestFileMappingConcurrentLookups(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("objects", 0755)

	m := NewFileMapping(fs, "objects")
	type pair struct {
		native plumbing.Hash
		compat plumbing.Hash
	}
	pairs := make([]pair, 64)
	for i := range pairs {
		pairs[i] = pair{
			native: plumbing.NewHash(fmt.Sprintf("%040x", i+1)),
			compat: plumbing.NewHash(fmt.Sprintf("%040x", i+1025)),
		}
		require.NoError(t, m.Add(pairs[i].native, pairs[i].compat))
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				p := pairs[(i+offset)%len(pairs)]
				gotCompat, err := m.NativeToCompat(p.native)
				if err != nil {
					errCh <- err
					return
				}
				if !gotCompat.Equal(p.compat) {
					errCh <- fmt.Errorf("native lookup mismatch: got %s want %s", gotCompat, p.compat)
					return
				}

				gotNative, err := m.CompatToNative(p.compat)
				if err != nil {
					errCh <- err
					return
				}
				if !gotNative.Equal(p.native) {
					errCh <- fmt.Errorf("compat lookup mismatch: got %s want %s", gotNative, p.native)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestFileMappingConcurrentAddAndLookup(t *testing.T) {
	fs := osfs.New(t.TempDir(), osfs.WithBoundOS())
	require.NoError(t, fs.MkdirAll("objects", 0755))

	m := NewFileMapping(fs, "objects")
	type pair struct {
		native plumbing.Hash
		compat plumbing.Hash
	}

	const (
		workers = 8
		perWork = 64
	)

	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < perWork; i++ {
				n := worker*perWork + i + 1
				p := pair{
					native: plumbing.NewHash(fmt.Sprintf("%040x", n)),
					compat: plumbing.NewHash(fmt.Sprintf("%040x", n+workers*perWork)),
				}
				if err := m.Add(p.native, p.compat); err != nil {
					errCh <- err
					return
				}

				gotCompat, err := m.NativeToCompat(p.native)
				if err != nil {
					errCh <- err
					return
				}
				if !gotCompat.Equal(p.compat) {
					errCh <- fmt.Errorf("native lookup mismatch: got %s want %s", gotCompat, p.compat)
					return
				}

				gotNative, err := m.CompatToNative(p.compat)
				if err != nil {
					errCh <- err
					return
				}
				if !gotNative.Equal(p.native) {
					errCh <- fmt.Errorf("compat lookup mismatch: got %s want %s", gotNative, p.native)
					return
				}
			}
		}(worker)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	assert.Equal(t, workers*perWork, m.Count())
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
