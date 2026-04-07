package compat_test

import (
	"io"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/compat"
	format "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslateStoredObjects(t *testing.T) {
	// Create a memory storage with SHA-1 objects.
	s := memory.NewStorage(memory.WithObjectFormat(format.SHA1))

	oh := plumbing.FromObjectFormat(format.SHA1)

	// Store a blob.
	blobContent := []byte("hello world\n")
	blob := plumbing.NewMemoryObject(oh)
	blob.SetType(plumbing.BlobObject)
	blob.Write(blobContent)
	blob.SetSize(int64(len(blobContent)))
	blobHash, err := s.ObjectStorage.SetEncodedObject(blob)
	require.NoError(t, err)

	// Store a tree referencing the blob.
	var treeContent []byte
	treeContent = append(treeContent, []byte("100644 hello.txt")...)
	treeContent = append(treeContent, 0x00)
	treeContent = append(treeContent, blobHash.Bytes()...)
	tree := plumbing.NewMemoryObject(oh)
	tree.SetType(plumbing.TreeObject)
	tree.Write(treeContent)
	tree.SetSize(int64(len(treeContent)))
	treeHash, err := s.ObjectStorage.SetEncodedObject(tree)
	require.NoError(t, err)

	// Store a commit referencing the tree.
	commitText := "tree " + treeHash.String() + "\n" +
		"author Test <t@t.com> 100 +0000\n" +
		"committer Test <t@t.com> 100 +0000\n" +
		"\n" +
		"test commit\n"
	commit := plumbing.NewMemoryObject(oh)
	commit.SetType(plumbing.CommitObject)
	commit.Write([]byte(commitText))
	commit.SetSize(int64(len(commitText)))
	commitHash, err := s.ObjectStorage.SetEncodedObject(commit)
	require.NoError(t, err)

	// Store a tag referencing the commit.
	tagText := "object " + commitHash.String() + "\n" +
		"type commit\n" +
		"tag v1.0\n" +
		"tagger Test <t@t.com> 100 +0000\n" +
		"\n" +
		"release\n"
	tag := plumbing.NewMemoryObject(oh)
	tag.SetType(plumbing.TagObject)
	tag.Write([]byte(tagText))
	tag.SetSize(int64(len(tagText)))
	tagHash, err := s.ObjectStorage.SetEncodedObject(tag)
	require.NoError(t, err)

	// Create a translator from SHA-1 (native) to SHA-256 (compat).
	m := compat.NewMemoryMapping()
	tr := compat.NewTranslator(compat.Formats{
		Native: format.SHA1,
		Compat: format.SHA256,
	}, m)

	// Translate all stored objects.
	err = compat.TranslateStoredObjects(s, tr)
	require.NoError(t, err)

	// Verify all 4 objects have mappings.
	assert.Equal(t, 4, m.Count())

	// Verify each object's mapping exists.
	for _, h := range []plumbing.Hash{blobHash, treeHash, commitHash, tagHash} {
		compatHash, err := m.NativeToCompat(h)
		require.NoError(t, err, "missing mapping for %s", h)
		assert.False(t, compatHash.IsZero())
	}
}

func TestTranslateStoredObjectsEmpty(t *testing.T) {
	s := memory.NewStorage()
	m := compat.NewMemoryMapping()
	tr := compat.NewTranslator(compat.Formats{
		Native: format.SHA1,
		Compat: format.SHA256,
	}, m)

	err := compat.TranslateStoredObjects(s, tr)
	require.NoError(t, err)
	assert.Equal(t, 0, m.Count())
}

func TestTranslateStoredObjectsTagOfTag(t *testing.T) {
	s := memory.NewStorage(memory.WithObjectFormat(format.SHA1))
	oh := plumbing.FromObjectFormat(format.SHA1)

	blobContent := []byte("hello world\n")
	blob := plumbing.NewMemoryObject(oh)
	blob.SetType(plumbing.BlobObject)
	blob.Write(blobContent)
	blob.SetSize(int64(len(blobContent)))
	blobHash, err := s.ObjectStorage.SetEncodedObject(blob)
	require.NoError(t, err)

	tag1Text := "object " + blobHash.String() + "\n" +
		"type blob\n" +
		"tag v1.0\n" +
		"tagger Test <t@t.com> 100 +0000\n" +
		"\n" +
		"release\n"
	tag1 := plumbing.NewMemoryObject(oh)
	tag1.SetType(plumbing.TagObject)
	tag1.Write([]byte(tag1Text))
	tag1.SetSize(int64(len(tag1Text)))
	tag1Hash, err := s.ObjectStorage.SetEncodedObject(tag1)
	require.NoError(t, err)

	tag2Text := "object " + tag1Hash.String() + "\n" +
		"type tag\n" +
		"tag latest\n" +
		"tagger Test <t@t.com> 200 +0000\n" +
		"\n" +
		"nested release\n"
	tag2 := plumbing.NewMemoryObject(oh)
	tag2.SetType(plumbing.TagObject)
	tag2.Write([]byte(tag2Text))
	tag2.SetSize(int64(len(tag2Text)))
	tag2Hash, err := s.ObjectStorage.SetEncodedObject(tag2)
	require.NoError(t, err)

	m := compat.NewMemoryMapping()
	tr := compat.NewTranslator(compat.Formats{
		Native: format.SHA1,
		Compat: format.SHA256,
	}, m)

	err = compat.TranslateStoredObjects(s, tr)
	require.NoError(t, err)

	_, err = m.NativeToCompat(tag2Hash)
	require.NoError(t, err)
}

func TestTranslateStoredObjectsMissingDependency(t *testing.T) {
	s := memory.NewStorage(memory.WithObjectFormat(format.SHA1))
	oh := plumbing.FromObjectFormat(format.SHA1)

	var treeContent []byte
	treeContent = append(treeContent, []byte("100644 orphan.txt")...)
	treeContent = append(treeContent, 0x00)
	treeContent = append(treeContent, plumbing.NewHash("1111111111111111111111111111111111111111").Bytes()...)

	tree := plumbing.NewMemoryObject(oh)
	tree.SetType(plumbing.TreeObject)
	tree.Write(treeContent)
	tree.SetSize(int64(len(treeContent)))
	_, err := s.ObjectStorage.SetEncodedObject(tree)
	require.NoError(t, err)

	m := compat.NewMemoryMapping()
	tr := compat.NewTranslator(compat.Formats{
		Native: format.SHA1,
		Compat: format.SHA256,
	}, m)

	err = compat.TranslateStoredObjects(s, tr)
	require.Error(t, err)
	assert.ErrorIs(t, err, compat.ErrMissingDependencyMapping)
}

func TestImportStoredObjects(t *testing.T) {
	src := memory.NewStorage(memory.WithObjectFormat(format.SHA1))
	dst := memory.NewStorage(
		memory.WithObjectFormat(format.SHA256),
		memory.WithCompatObjectFormat(format.SHA1),
	)

	oh := plumbing.FromObjectFormat(format.SHA1)

	blobContent := []byte("hello world\n")
	blob := plumbing.NewMemoryObject(oh)
	blob.SetType(plumbing.BlobObject)
	blob.Write(blobContent)
	blob.SetSize(int64(len(blobContent)))
	blobHash, err := src.SetEncodedObject(blob)
	require.NoError(t, err)

	var treeContent []byte
	treeContent = append(treeContent, []byte("100644 hello.txt")...)
	treeContent = append(treeContent, 0x00)
	treeContent = append(treeContent, blobHash.Bytes()...)
	tree := plumbing.NewMemoryObject(oh)
	tree.SetType(plumbing.TreeObject)
	tree.Write(treeContent)
	tree.SetSize(int64(len(treeContent)))
	treeHash, err := src.SetEncodedObject(tree)
	require.NoError(t, err)

	commitText := "tree " + treeHash.String() + "\n" +
		"author Test <t@t.com> 100 +0000\n" +
		"committer Test <t@t.com> 100 +0000\n" +
		"\n" +
		"test commit\n"
	commit := plumbing.NewMemoryObject(oh)
	commit.SetType(plumbing.CommitObject)
	commit.Write([]byte(commitText))
	commit.SetSize(int64(len(commitText)))
	commitHash, err := src.SetEncodedObject(commit)
	require.NoError(t, err)

	tr := dst.Translator()
	require.NotNil(t, tr)

	err = compat.ImportStoredObjects(src, dst, tr)
	require.NoError(t, err)

	for _, compatHash := range []plumbing.Hash{blobHash, treeHash, commitHash} {
		nativeHash, err := tr.Mapping().CompatToNative(compatHash)
		require.NoError(t, err)
		assert.False(t, nativeHash.IsZero())

		_, err = dst.EncodedObject(plumbing.AnyObject, compatHash)
		require.NoError(t, err)
	}

	nativeCommitHash, err := tr.Mapping().CompatToNative(commitHash)
	require.NoError(t, err)
	commitObj, err := dst.ObjectStorage.EncodedObject(plumbing.CommitObject, nativeCommitHash)
	require.NoError(t, err)

	r, err := commitObj.Reader()
	require.NoError(t, err)
	defer r.Close()

	content, err := io.ReadAll(r)
	require.NoError(t, err)

	nativeTreeHash, err := tr.Mapping().CompatToNative(treeHash)
	require.NoError(t, err)
	assert.Contains(t, string(content), "tree "+nativeTreeHash.String())
	assert.NotContains(t, string(content), "tree "+treeHash.String())
}

func TestImportStoredObjectsTagOfTag(t *testing.T) {
	src := memory.NewStorage(memory.WithObjectFormat(format.SHA1))
	dst := memory.NewStorage(
		memory.WithObjectFormat(format.SHA256),
		memory.WithCompatObjectFormat(format.SHA1),
	)

	oh := plumbing.FromObjectFormat(format.SHA1)

	blobContent := []byte("hello world\n")
	blob := plumbing.NewMemoryObject(oh)
	blob.SetType(plumbing.BlobObject)
	blob.Write(blobContent)
	blob.SetSize(int64(len(blobContent)))
	blobHash, err := src.SetEncodedObject(blob)
	require.NoError(t, err)

	tag1Text := "object " + blobHash.String() + "\n" +
		"type blob\n" +
		"tag v1.0\n" +
		"tagger Test <t@t.com> 100 +0000\n" +
		"\n" +
		"release\n"
	tag1 := plumbing.NewMemoryObject(oh)
	tag1.SetType(plumbing.TagObject)
	tag1.Write([]byte(tag1Text))
	tag1.SetSize(int64(len(tag1Text)))
	tag1Hash, err := src.SetEncodedObject(tag1)
	require.NoError(t, err)

	tag2Text := "object " + tag1Hash.String() + "\n" +
		"type tag\n" +
		"tag latest\n" +
		"tagger Test <t@t.com> 200 +0000\n" +
		"\n" +
		"nested release\n"
	tag2 := plumbing.NewMemoryObject(oh)
	tag2.SetType(plumbing.TagObject)
	tag2.Write([]byte(tag2Text))
	tag2.SetSize(int64(len(tag2Text)))
	tag2Hash, err := src.SetEncodedObject(tag2)
	require.NoError(t, err)

	tr := dst.Translator()
	require.NotNil(t, tr)

	err = compat.ImportStoredObjects(src, dst, tr)
	require.NoError(t, err)

	nativeTag2Hash, err := tr.Mapping().CompatToNative(tag2Hash)
	require.NoError(t, err)
	assert.False(t, nativeTag2Hash.IsZero())

	obj, err := dst.ObjectStorage.EncodedObject(plumbing.TagObject, nativeTag2Hash)
	require.NoError(t, err)
	r, err := obj.Reader()
	require.NoError(t, err)
	defer r.Close()

	content, err := io.ReadAll(r)
	require.NoError(t, err)

	nativeTag1Hash, err := tr.Mapping().CompatToNative(tag1Hash)
	require.NoError(t, err)
	assert.Contains(t, string(content), "object "+nativeTag1Hash.String())
}
