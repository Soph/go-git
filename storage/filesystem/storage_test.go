package filesystem_test

import (
	"os"
	"testing"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/osfs"
	fixtures "github.com/go-git/go-git-fixtures/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/plumbing/compat"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/go-git/go-git/v6/storage/filesystem"
	xstorage "github.com/go-git/go-git/v6/x/storage"
)

func mustDotGit(t testing.TB, f *fixtures.Fixture, opts ...fixtures.Option) billy.Filesystem {
	t.Helper()
	fs, err := f.DotGit(opts...)
	require.NoError(t, err)
	return fs
}

var (
	fs  = memfs.New()
	sto = filesystem.NewStorage(fs, cache.NewObjectLRUDefault())

	// Ensure interfaces are implemented.
	_ storer.EncodedObjectStorer = sto
	_ storer.IndexStorer         = sto
	_ storer.ReferenceStorer     = sto
	_ storer.ShallowStorer       = sto
	_ storer.DeltaObjectStorer   = sto
	_ storer.PackfileWriter      = sto
	_ xstorage.ExtensionChecker  = sto
)

func TestFilesystem(t *testing.T) {
	t.Parallel()
	assert.Same(t, fs, sto.Filesystem())
}

func TestNewStorageShouldNotAddAnyContentsToDir(t *testing.T) {
	t.Parallel()
	fs := osfs.New(t.TempDir())

	sto := filesystem.NewStorageWithOptions(
		fs,
		cache.NewObjectLRUDefault(),
		filesystem.Options{ExclusiveAccess: true})
	assert.NotNil(t, sto)

	fis, err := fs.ReadDir("/")
	assert.NoError(t, err)
	assert.Len(t, fis, 0)
}

func TestSetObjectFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		initialFormat formatcfg.ObjectFormat
		targetFormat  formatcfg.ObjectFormat
		wantErr       bool
		errContains   string
	}{
		{
			name:          "set SHA1 on empty storage",
			initialFormat: formatcfg.UnsetObjectFormat,
			targetFormat:  formatcfg.SHA1,
			wantErr:       false,
		},
		{
			name:          "set SHA256 on empty storage",
			initialFormat: formatcfg.UnsetObjectFormat,
			targetFormat:  formatcfg.SHA256,
			wantErr:       false,
		},
		{
			name:          "set SHA1 when already SHA1",
			initialFormat: formatcfg.SHA1,
			targetFormat:  formatcfg.SHA1,
			wantErr:       false,
		},
		{
			name:          "set SHA256 when already SHA256",
			initialFormat: formatcfg.SHA256,
			targetFormat:  formatcfg.SHA256,
			wantErr:       false,
		},
		{
			name:          "change from SHA1 to SHA256",
			initialFormat: formatcfg.SHA1,
			targetFormat:  formatcfg.SHA256,
			wantErr:       false,
		},
		{
			name:          "change from SHA256 to SHA1",
			initialFormat: formatcfg.SHA256,
			targetFormat:  formatcfg.SHA1,
			wantErr:       false,
		},
		{
			name:          "invalid object format",
			initialFormat: formatcfg.UnsetObjectFormat,
			targetFormat:  formatcfg.ObjectFormat("invalid"),
			wantErr:       true,
			errContains:   "invalid object format",
		},
		{
			name:          "empty string object format",
			initialFormat: formatcfg.UnsetObjectFormat,
			targetFormat:  formatcfg.ObjectFormat(""),
			wantErr:       true,
			errContains:   "invalid object format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fs := osfs.New(t.TempDir())
			sto := filesystem.NewStorageWithOptions(
				fs,
				cache.NewObjectLRUDefault(),
				filesystem.Options{ObjectFormat: tt.initialFormat},
			)
			require.NoError(t, sto.Init())

			err := sto.SetObjectFormat(tt.targetFormat)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewStorageWithOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		fs               billy.Filesystem
		inObjectFormat   formatcfg.ObjectFormat
		wantObjectFormat formatcfg.ObjectFormat
	}{
		{
			name:             "existing SHA1 (unset) repo, unset opts format",
			fs:               mustDotGit(t, fixtures.ByTag(".git").One()),
			inObjectFormat:   formatcfg.UnsetObjectFormat,
			wantObjectFormat: formatcfg.UnsetObjectFormat,
		},
		{
			name:             "existing SHA1 repo, unset opts format",
			fs:               getExplicitSHA1(t),
			inObjectFormat:   formatcfg.UnsetObjectFormat,
			wantObjectFormat: formatcfg.SHA1,
		},
		{
			name:             "existing SHA256 repo, unset opts format",
			fs:               mustDotGit(t, fixtures.ByTag(".git-sha256").One()),
			inObjectFormat:   formatcfg.UnsetObjectFormat,
			wantObjectFormat: formatcfg.SHA256,
		},
		{
			name:             "existing SHA1 (unset) repo, SHA1 opts format",
			fs:               mustDotGit(t, fixtures.ByTag(".git").One()),
			inObjectFormat:   formatcfg.SHA1,
			wantObjectFormat: formatcfg.UnsetObjectFormat,
		},
		{
			name:             "existing SHA1 repo, SHA1 opts format",
			fs:               getExplicitSHA1(t),
			inObjectFormat:   formatcfg.SHA1,
			wantObjectFormat: formatcfg.SHA1,
		},
		{
			name:             "existing SHA256 repo, SHA256 opts format",
			fs:               mustDotGit(t, fixtures.ByTag(".git-sha256").One()),
			inObjectFormat:   formatcfg.SHA256,
			wantObjectFormat: formatcfg.SHA256,
		},
		{
			name:             "SHA256 opts format conflicts with existing SHA1 config",
			fs:               mustDotGit(t, fixtures.ByTag(".git").One()),
			inObjectFormat:   formatcfg.SHA256,
			wantObjectFormat: formatcfg.UnsetObjectFormat,
		},
		{
			name:             "existing SHA256 repo, SHA1 opts format",
			fs:               mustDotGit(t, fixtures.ByTag(".git-sha256").One()),
			inObjectFormat:   formatcfg.SHA1,
			wantObjectFormat: formatcfg.SHA256,
		},
		{
			name:             "empty fs, no opts format",
			fs:               osfs.New(t.TempDir()),
			inObjectFormat:   formatcfg.UnsetObjectFormat,
			wantObjectFormat: formatcfg.UnsetObjectFormat,
		},
		{
			name:             "empty fs, SHA1 opts format",
			fs:               osfs.New(t.TempDir()),
			inObjectFormat:   formatcfg.SHA1,
			wantObjectFormat: formatcfg.UnsetObjectFormat,
		},
		{
			name:             "empty fs, SHA256 opts format",
			fs:               osfs.New(t.TempDir()),
			inObjectFormat:   formatcfg.SHA256,
			wantObjectFormat: formatcfg.SHA256,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sto := filesystem.NewStorageWithOptions(
				tt.fs,
				cache.NewObjectLRUDefault(),
				filesystem.Options{ObjectFormat: tt.inObjectFormat},
			)

			cfg, err := sto.Config()
			require.NoError(t, err)

			assert.Equal(t, tt.wantObjectFormat, cfg.Extensions.ObjectFormat)
		})
	}
}

func TestSetObjectFormatWithExistingPackfiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		tag          string
		targetFormat formatcfg.ObjectFormat
	}{
		{
			name:         "change to SHA256 with existing packfiles",
			tag:          ".git",
			targetFormat: formatcfg.SHA256,
		},
		{
			name:         "set same format SHA1 with existing packfiles",
			tag:          ".git",
			targetFormat: formatcfg.SHA1,
		},
		{
			name:         "change to SHA1 with existing SHA256 packfiles",
			tag:          ".git-sha256",
			targetFormat: formatcfg.SHA1,
		},
		{
			name:         "set same format SHA256 with existing packfiles",
			tag:          ".git-sha256",
			targetFormat: formatcfg.SHA256,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fs, err := fixtures.ByTag(tt.tag).One().DotGit()
			require.NoError(t, err)
			sto := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())

			packs, err := sto.ObjectPacks()
			require.NoError(t, err)
			require.Len(t, packs, 1)

			err = sto.SetObjectFormat(tt.targetFormat)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), "cannot change object format")
		})
	}
}

func TestSupportsExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		ext   string
		value string
		want  bool
	}{
		{
			name:  "objectformat with sha1",
			ext:   "objectformat",
			value: "sha1",
			want:  true,
		},
		{
			name:  "objectformat with sha256",
			ext:   "objectformat",
			value: "sha256",
			want:  true,
		},
		{
			name:  "objectformat with empty string",
			ext:   "objectformat",
			value: "",
			want:  true,
		},
		{
			name:  "objectformat with unsupported value",
			ext:   "objectformat",
			value: "sha512",
			want:  false,
		},
		{
			name:  "unsupported extension name",
			ext:   "noop",
			value: "sha1",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sto := filesystem.NewStorage(memfs.New(), cache.NewObjectLRUDefault())
			got := sto.SupportsExtension(tt.ext, tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSetEncodedObjectPersistsCompatMapping(t *testing.T) {
	t.Parallel()

	fs := memfs.New()
	sto := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	require.NoError(t, sto.Init())

	cfg, err := sto.Config()
	require.NoError(t, err)
	cfg.Core.RepositoryFormatVersion = formatcfg.Version1
	cfg.Extensions.ObjectFormat = formatcfg.SHA256
	cfg.Extensions.CompatObjectFormat = formatcfg.SHA1
	require.NoError(t, sto.SetConfig(cfg))

	sto = filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	tp, ok := any(sto).(xstorage.CompatTranslatorProvider)
	require.True(t, ok)
	require.NotNil(t, tp.Translator())

	obj := sto.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	content := []byte("hello compat\n")
	obj.SetSize(int64(len(content)))
	w, err := obj.Writer()
	require.NoError(t, err)
	_, err = w.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	nativeHash, err := sto.SetEncodedObject(obj)
	require.NoError(t, err)

	compatHash, err := tp.Translator().Mapping().NativeToCompat(nativeHash)
	require.NoError(t, err)
	assert.False(t, compatHash.IsZero())

	reopened := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	compatObj, err := reopened.EncodedObject(plumbing.BlobObject, compatHash)
	require.NoError(t, err)
	assert.Equal(t, nativeHash, compatObj.Hash())
}

func getExplicitSHA1(t testing.TB) billy.Filesystem {
	fs := osfs.New(t.TempDir())
	st := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	cfg, err := st.Config()
	require.NoError(t, err)

	cfg.Extensions.ObjectFormat = formatcfg.SHA1
	cfg.Core.RepositoryFormatVersion = formatcfg.Version1
	err = st.SetConfig(cfg)
	require.NoError(t, err)

	return fs
}

func TestCloseClosesCompatMapping(t *testing.T) {
	t.Parallel()

	base := memfs.New()
	fs := &trackingOpenFileFS{Filesystem: base, failPath: "objects/" + "loose-object-idx"}
	sto := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	require.NoError(t, sto.Init())

	cfg, err := sto.Config()
	require.NoError(t, err)
	cfg.Core.RepositoryFormatVersion = formatcfg.Version1
	cfg.Extensions.ObjectFormat = formatcfg.SHA256
	cfg.Extensions.CompatObjectFormat = formatcfg.SHA1
	require.NoError(t, sto.SetConfig(cfg))

	sto = filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	obj := sto.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	content := []byte("hello compat\n")
	obj.SetSize(int64(len(content)))
	w, err := obj.Writer()
	require.NoError(t, err)
	_, err = w.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	_, err = sto.SetEncodedObject(obj)
	require.NoError(t, err)

	require.NotNil(t, fs.lastFile)
	assert.Equal(t, 0, fs.lastFile.closeCount)
	require.NoError(t, sto.Close())
	assert.Equal(t, 1, fs.lastFile.closeCount)
}

type trackingOpenFileFS struct {
	billy.Filesystem
	failPath string
	lastFile *trackingFile
}

func (fs *trackingOpenFileFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	f, err := fs.Filesystem.OpenFile(filename, flag, perm)
	if err != nil {
		return nil, err
	}
	if filename == fs.failPath && flag&(os.O_APPEND|os.O_WRONLY) == (os.O_APPEND|os.O_WRONLY) {
		fs.lastFile = &trackingFile{File: f}
		return fs.lastFile, nil
	}
	return f, nil
}

type trackingFile struct {
	billy.File
	closeCount int
}

func (f *trackingFile) Close() error {
	f.closeCount++
	return f.File.Close()
}

func TestSetEncodedObjectCompatTranslationFailureIsFatal(t *testing.T) {
	t.Parallel()

	fs := memfs.New()
	sto := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	require.NoError(t, sto.Init())

	cfg, err := sto.Config()
	require.NoError(t, err)
	cfg.Core.RepositoryFormatVersion = formatcfg.Version1
	cfg.Extensions.ObjectFormat = formatcfg.SHA256
	cfg.Extensions.CompatObjectFormat = formatcfg.SHA1
	require.NoError(t, sto.SetConfig(cfg))

	sto = filesystem.NewStorage(fs, cache.NewObjectLRUDefault())

	obj := sto.NewEncodedObject()
	obj.SetType(plumbing.CommitObject)
	content := []byte(
		"tree 1111111111111111111111111111111111111111111111111111111111111111\n" +
			"author A <a@b.c> 100 +0000\n" +
			"committer A <a@b.c> 100 +0000\n\nbroken\n",
	)
	obj.SetSize(int64(len(content)))
	w, err := obj.Writer()
	require.NoError(t, err)
	_, err = w.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	h, err := sto.SetEncodedObject(obj)
	require.Error(t, err)
	assert.ErrorIs(t, err, compat.ErrMissingDependencyMapping)
	assert.False(t, h.IsZero())

	_, lookupErr := sto.ObjectStorage.EncodedObject(plumbing.CommitObject, h)
	require.NoError(t, lookupErr)

	tp, ok := any(sto).(xstorage.CompatTranslatorProvider)
	require.True(t, ok)
	_, lookupErr = tp.Translator().Mapping().NativeToCompat(h)
	assert.ErrorIs(t, lookupErr, plumbing.ErrObjectNotFound)
}

func TestSetEncodedObjectCompatImportRequiresBackfill(t *testing.T) {
	t.Parallel()

	fs := memfs.New()
	sto := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	require.NoError(t, sto.Init())

	cfg, err := sto.Config()
	require.NoError(t, err)
	cfg.Core.RepositoryFormatVersion = formatcfg.Version1
	cfg.Extensions.ObjectFormat = formatcfg.SHA256
	cfg.Extensions.CompatObjectFormat = formatcfg.SHA1
	require.NoError(t, sto.SetConfig(cfg))

	sto = filesystem.NewStorage(fs, cache.NewObjectLRUDefault())

	blob := sto.NewEncodedObject()
	blob.SetType(plumbing.BlobObject)
	blobContent := []byte("data")
	blob.SetSize(int64(len(blobContent)))
	w, err := blob.Writer()
	require.NoError(t, err)
	_, err = w.Write(blobContent)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	done := sto.BeginCompatObjectImport()
	blobHash, err := sto.SetEncodedObject(blob)
	require.NoError(t, err)

	tree := sto.NewEncodedObject()
	tree.SetType(plumbing.TreeObject)
	var treeContent []byte
	treeContent = append(treeContent, []byte("100644 f.txt")...)
	treeContent = append(treeContent, 0x00)
	treeContent = append(treeContent, blobHash.Bytes()...)
	tree.SetSize(int64(len(treeContent)))
	w, err = tree.Writer()
	require.NoError(t, err)
	_, err = w.Write(treeContent)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	treeHash, err := sto.SetEncodedObject(tree)
	require.NoError(t, err)

	commit := sto.NewEncodedObject()
	commit.SetType(plumbing.CommitObject)
	commitContent := []byte(
		"tree " + treeHash.String() + "\n" +
			"author A <a@b.c> 100 +0000\n" +
			"committer A <a@b.c> 100 +0000\n\nok\n",
	)
	commit.SetSize(int64(len(commitContent)))
	w, err = commit.Writer()
	require.NoError(t, err)
	_, err = w.Write(commitContent)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	commitHash, err := sto.SetEncodedObject(commit)
	require.NoError(t, err)
	done()

	tp, ok := any(sto).(xstorage.CompatTranslatorProvider)
	require.True(t, ok)
	_, err = tp.Translator().Mapping().NativeToCompat(commitHash)
	assert.ErrorIs(t, err, plumbing.ErrObjectNotFound)

	require.NoError(t, compat.TranslateStoredObjects(sto, tp.Translator()))

	compatHash, err := tp.Translator().Mapping().NativeToCompat(commitHash)
	require.NoError(t, err)
	assert.False(t, compatHash.IsZero())
}
