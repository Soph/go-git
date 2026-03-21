//go:build interop

package interop_test

import (
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/stretchr/testify/require"
)

func TestBranch_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)
	cli.seed()
	cli.git("branch", "feature-a")
	cli.git("branch", "feature-b")

	branches := snapshotBranches(t, cli.open())
	headHash := cli.git("rev-parse", "HEAD")
	require.Equal(t, headHash, branches["main"])
	require.Equal(t, headHash, branches["feature-a"])
	require.Equal(t, headHash, branches["feature-b"])
	require.Len(t, branches, 3)
}

func TestBranch_GoGitThenGit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)

	r.seed()

	repo := r.open()
	head, err := repo.Head()
	require.NoError(t, err)

	for _, name := range []string{"feature-a", "feature-b"} {
		err = repo.Storer.SetReference(
			plumbing.NewHashReference(plumbing.NewBranchReferenceName(name), head.Hash()),
		)
		require.NoError(t, err)
	}

	r.fsck()
	out := r.git("branch")
	require.Contains(t, out, "feature-a")
	require.Contains(t, out, "feature-b")
}

func TestLightweightTag_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)
	cli.seed()
	cli.git("tag", "v1.0.0")

	snap := snapshotTag(t, cli.open(), "v1.0.0")
	require.Equal(t, "lightweight", snap.Kind)
	require.Equal(t, cli.git("rev-parse", "HEAD"), snap.TargetHash)
	require.Equal(t, "v1.0.0", snap.Name)
}

func TestLightweightTag_GoGitThenGit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)

	r.seed()

	repo := r.open()
	head, err := repo.Head()
	require.NoError(t, err)

	_, err = repo.CreateTag("v1.0.0", head.Hash(), nil)
	require.NoError(t, err)

	r.fsck()
	out := r.git("tag", "-l")
	require.Contains(t, out, "v1.0.0")

	tagHash := r.git("rev-parse", "v1.0.0")
	require.Equal(t, head.Hash().String(), tagHash)
}

func TestAnnotatedTag_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)
	cli.seed()
	cli.git("tag", "-a", "v2.0.0", "-m", "release v2")

	snap := snapshotTag(t, cli.open(), "v2.0.0")
	require.Equal(t, "annotated", snap.Kind)
	require.Equal(t, cli.git("rev-parse", "HEAD"), snap.TargetHash)
	require.Equal(t, "v2.0.0", snap.Name)
	require.Equal(t, "release v2", snap.Message)
}

func TestAnnotatedTag_GoGitThenGit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)

	r.seed()

	repo := r.open()
	head, err := repo.Head()
	require.NoError(t, err)

	sig := defaultSignature()
	_, err = repo.CreateTag("v2.0.0", head.Hash(), &git.CreateTagOptions{
		Tagger:  sig,
		Message: "release v2",
	})
	require.NoError(t, err)

	r.fsck()
	out := r.git("tag", "-l", "-n1")
	require.Contains(t, out, "v2.0.0")
	require.Contains(t, out, "release v2")

	// Verify it's annotated and points at the right commit.
	require.Equal(t, "tag", r.git("cat-file", "-t", "v2.0.0"))
	require.Equal(t, head.Hash().String(), r.git("rev-parse", "v2.0.0^{}"))
}
