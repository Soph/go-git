package compat

import (
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
	format "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTranslator() (*Translator, *MemoryMapping) {
	m := NewMemoryMapping()
	f := Formats{
		Native: format.SHA1,
		Compat: format.SHA256,
	}
	return NewTranslator(f, m), m
}

func makeEncodedObject(t *testing.T, objType plumbing.ObjectType, content []byte, f format.ObjectFormat) plumbing.EncodedObject {
	t.Helper()
	hasher := plumbing.FromObjectFormat(f)
	obj := plumbing.NewMemoryObject(hasher)
	obj.SetType(objType)
	obj.SetSize(int64(len(content)))
	w, err := obj.Writer()
	require.NoError(t, err)
	_, err = w.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return obj
}

func TestTranslateBlob(t *testing.T) {
	tr, m := newTestTranslator()

	blobContent := []byte("hello world\n")
	obj := makeEncodedObject(t, plumbing.BlobObject, blobContent, format.SHA1)

	compatHash, err := tr.TranslateObject(obj)
	require.NoError(t, err)

	// The compat hash should be the SHA-256 of the same content.
	expectedHash, err := tr.ComputeCompatHash(plumbing.BlobObject, blobContent)
	require.NoError(t, err)
	assert.True(t, compatHash.Equal(expectedHash), "compat hash mismatch: got %s, want %s", compatHash, expectedHash)

	// Mapping should be recorded.
	assert.Equal(t, 1, m.Count())
	got, err := m.NativeToCompat(obj.Hash())
	require.NoError(t, err)
	assert.True(t, got.Equal(compatHash))
}

func TestTranslateTree(t *testing.T) {
	tr, m := newTestTranslator()

	// First, create a blob and translate it so its mapping exists.
	blobContent := []byte("file content")
	blobObj := makeEncodedObject(t, plumbing.BlobObject, blobContent, format.SHA1)
	_, err := tr.TranslateObject(blobObj)
	require.NoError(t, err)

	// Build a tree with one entry pointing to the blob.
	// Tree entry format: "<mode> <name>\0<20-byte-hash>"
	var treeContent []byte
	treeContent = append(treeContent, []byte("100644 test.txt")...)
	treeContent = append(treeContent, 0x00)
	treeContent = append(treeContent, blobObj.Hash().Bytes()...)

	treeObj := makeEncodedObject(t, plumbing.TreeObject, treeContent, format.SHA1)

	compatHash, err := tr.TranslateObject(treeObj)
	require.NoError(t, err)

	// Verify the mapping was recorded.
	assert.Equal(t, 2, m.Count()) // blob + tree
	got, err := m.NativeToCompat(treeObj.Hash())
	require.NoError(t, err)
	assert.True(t, got.Equal(compatHash))

	// Verify the compat hash is different from the native hash.
	assert.False(t, treeObj.Hash().Equal(compatHash))
}

func TestTranslateCommit(t *testing.T) {
	tr, m := newTestTranslator()

	// Create and translate a blob, then a tree pointing to it.
	blobContent := []byte("content")
	blobObj := makeEncodedObject(t, plumbing.BlobObject, blobContent, format.SHA1)
	_, err := tr.TranslateObject(blobObj)
	require.NoError(t, err)

	var treeContent []byte
	treeContent = append(treeContent, []byte("100644 file.txt")...)
	treeContent = append(treeContent, 0x00)
	treeContent = append(treeContent, blobObj.Hash().Bytes()...)
	treeObj := makeEncodedObject(t, plumbing.TreeObject, treeContent, format.SHA1)
	_, err = tr.TranslateObject(treeObj)
	require.NoError(t, err)

	// Build a commit referencing the tree.
	commitText := "tree " + treeObj.Hash().String() + "\n" +
		"author Test User <test@example.com> 1234567890 +0000\n" +
		"committer Test User <test@example.com> 1234567890 +0000\n" +
		"\n" +
		"Initial commit\n"

	commitObj := makeEncodedObject(t, plumbing.CommitObject, []byte(commitText), format.SHA1)
	compatHash, err := tr.TranslateObject(commitObj)
	require.NoError(t, err)

	assert.Equal(t, 3, m.Count()) // blob + tree + commit
	got, err := m.NativeToCompat(commitObj.Hash())
	require.NoError(t, err)
	assert.True(t, got.Equal(compatHash))
}

func TestTranslateCommitWithParents(t *testing.T) {
	tr, _ := newTestTranslator()

	// Create blob -> tree -> commit1 (root) -> commit2 (child).
	blobObj := makeEncodedObject(t, plumbing.BlobObject, []byte("data"), format.SHA1)
	_, err := tr.TranslateObject(blobObj)
	require.NoError(t, err)

	var treeContent []byte
	treeContent = append(treeContent, []byte("100644 f.txt")...)
	treeContent = append(treeContent, 0x00)
	treeContent = append(treeContent, blobObj.Hash().Bytes()...)
	treeObj := makeEncodedObject(t, plumbing.TreeObject, treeContent, format.SHA1)
	_, err = tr.TranslateObject(treeObj)
	require.NoError(t, err)

	// Root commit (no parent).
	commit1Text := "tree " + treeObj.Hash().String() + "\n" +
		"author A <a@b.c> 100 +0000\n" +
		"committer A <a@b.c> 100 +0000\n" +
		"\n" +
		"root\n"
	commit1Obj := makeEncodedObject(t, plumbing.CommitObject, []byte(commit1Text), format.SHA1)
	_, err = tr.TranslateObject(commit1Obj)
	require.NoError(t, err)

	// Child commit with parent.
	commit2Text := "tree " + treeObj.Hash().String() + "\n" +
		"parent " + commit1Obj.Hash().String() + "\n" +
		"author A <a@b.c> 200 +0000\n" +
		"committer A <a@b.c> 200 +0000\n" +
		"\n" +
		"child\n"
	commit2Obj := makeEncodedObject(t, plumbing.CommitObject, []byte(commit2Text), format.SHA1)
	compatHash, err := tr.TranslateObject(commit2Obj)
	require.NoError(t, err)

	// Verify the compat hash was computed and recorded.
	assert.False(t, compatHash.IsZero())
}

func TestTranslateCommitWithMergeTag(t *testing.T) {
	tr, _ := newTestTranslator()

	blobObj := makeEncodedObject(t, plumbing.BlobObject, []byte("data"), format.SHA1)
	_, err := tr.TranslateObject(blobObj)
	require.NoError(t, err)

	var treeContent []byte
	treeContent = append(treeContent, []byte("100644 f.txt")...)
	treeContent = append(treeContent, 0x00)
	treeContent = append(treeContent, blobObj.Hash().Bytes()...)
	treeObj := makeEncodedObject(t, plumbing.TreeObject, treeContent, format.SHA1)
	_, err = tr.TranslateObject(treeObj)
	require.NoError(t, err)

	rootText := "tree " + treeObj.Hash().String() + "\n" +
		"author A <a@b.c> 100 +0000\n" +
		"committer A <a@b.c> 100 +0000\n" +
		"\n" +
		"root\n"
	rootObj := makeEncodedObject(t, plumbing.CommitObject, []byte(rootText), format.SHA1)
	_, err = tr.TranslateObject(rootObj)
	require.NoError(t, err)

	rootCompat, err := tr.Mapping().NativeToCompat(rootObj.Hash())
	require.NoError(t, err)

	mergeTag := "object " + rootObj.Hash().String() + "\n" +
		"type commit\n" +
		"tag merged\n" +
		"tagger A <a@b.c> 150 +0000\n" +
		"\n" +
		"merge tag message\n"
	commitText := "tree " + treeObj.Hash().String() + "\n" +
		"parent " + rootObj.Hash().String() + "\n" +
		"author A <a@b.c> 200 +0000\n" +
		"committer A <a@b.c> 200 +0000\n" +
		"mergetag object " + rootObj.Hash().String() + "\n" +
		" type commit\n" +
		" tag merged\n" +
		" tagger A <a@b.c> 150 +0000\n" +
		" \n" +
		" merge tag message\n" +
		"\n" +
		"child\n"
	commitObj := makeEncodedObject(t, plumbing.CommitObject, []byte(commitText), format.SHA1)

	compatHash, err := tr.TranslateObject(commitObj)
	require.NoError(t, err)
	assert.False(t, compatHash.IsZero())

	translatedContent, err := tr.ReverseTranslateContent(plumbing.CommitObject, []byte(commitText))
	require.NoError(t, err)
	assert.Contains(t, string(translatedContent), "mergetag object "+rootCompat.String())
	assert.Contains(t, string(translatedContent), "parent "+rootCompat.String())

	roundTrip, err := tr.reverseTranslateCommit(translatedContent)
	require.NoError(t, err)
	assert.Equal(t, commitText, string(roundTrip))

	translatedMergeTag, err := tr.rewriteTagContent([]byte(mergeTag), false)
	require.NoError(t, err)
	assert.Contains(t, string(translatedMergeTag), "object "+rootCompat.String())
}

func TestTranslateTag(t *testing.T) {
	tr, m := newTestTranslator()

	// Create a blob to tag.
	blobObj := makeEncodedObject(t, plumbing.BlobObject, []byte("tagged content"), format.SHA1)
	_, err := tr.TranslateObject(blobObj)
	require.NoError(t, err)

	tagText := "object " + blobObj.Hash().String() + "\n" +
		"type blob\n" +
		"tag v1.0\n" +
		"tagger Test <t@t.com> 100 +0000\n" +
		"\n" +
		"Release v1.0\n"
	tagObj := makeEncodedObject(t, plumbing.TagObject, []byte(tagText), format.SHA1)
	compatHash, err := tr.TranslateObject(tagObj)
	require.NoError(t, err)

	assert.Equal(t, 2, m.Count()) // blob + tag
	got, err := m.NativeToCompat(tagObj.Hash())
	require.NoError(t, err)
	assert.True(t, got.Equal(compatHash))
}

func TestTranslateTagOfTag(t *testing.T) {
	tr, m := newTestTranslator()

	blobObj := makeEncodedObject(t, plumbing.BlobObject, []byte("tagged content"), format.SHA1)
	_, err := tr.TranslateObject(blobObj)
	require.NoError(t, err)

	tag1Text := "object " + blobObj.Hash().String() + "\n" +
		"type blob\n" +
		"tag v1.0\n" +
		"tagger Test <t@t.com> 100 +0000\n" +
		"\n" +
		"Release v1.0\n"
	tag1Obj := makeEncodedObject(t, plumbing.TagObject, []byte(tag1Text), format.SHA1)
	_, err = tr.TranslateObject(tag1Obj)
	require.NoError(t, err)

	tag2Text := "object " + tag1Obj.Hash().String() + "\n" +
		"type tag\n" +
		"tag latest\n" +
		"tagger Test <t@t.com> 200 +0000\n" +
		"\n" +
		"Nested tag\n"
	tag2Obj := makeEncodedObject(t, plumbing.TagObject, []byte(tag2Text), format.SHA1)
	compatHash, err := tr.TranslateObject(tag2Obj)
	require.NoError(t, err)

	got, err := m.NativeToCompat(tag2Obj.Hash())
	require.NoError(t, err)
	assert.True(t, got.Equal(compatHash))
}

func TestTranslateTreeMissingMapping(t *testing.T) {
	tr, _ := newTestTranslator()

	// Build a tree entry with a hash that has no mapping.
	fakeHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	var treeContent []byte
	treeContent = append(treeContent, []byte("100644 orphan.txt")...)
	treeContent = append(treeContent, 0x00)
	treeContent = append(treeContent, fakeHash.Bytes()...)

	treeObj := makeEncodedObject(t, plumbing.TreeObject, treeContent, format.SHA1)
	_, err := tr.TranslateObject(treeObj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no compat mapping")
}

func TestTranslateCommitMissingTreeMapping(t *testing.T) {
	tr, _ := newTestTranslator()

	fakeTreeHash := plumbing.NewHash("2222222222222222222222222222222222222222")
	commitText := "tree " + fakeTreeHash.String() + "\n" +
		"author A <a@b.c> 100 +0000\n" +
		"committer A <a@b.c> 100 +0000\n" +
		"\n" +
		"test\n"

	commitObj := makeEncodedObject(t, plumbing.CommitObject, []byte(commitText), format.SHA1)
	_, err := tr.TranslateObject(commitObj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no compat mapping")
}
