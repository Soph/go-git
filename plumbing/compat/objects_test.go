package compat_test

import (
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
