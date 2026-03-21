//go:build interop

package interop_test

import (
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/stretchr/testify/require"
)

func TestMergeFF_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)

	cli.writeFile("a.txt", "a\n")
	cli.git("add", "a.txt")
	cli.git("commit", "-m", "base")

	cli.git("checkout", "-b", "feature")
	cli.writeFile("b.txt", "b\n")
	cli.git("add", "b.txt")
	cli.git("commit", "-m", "feature work")
	featureHash := cli.git("rev-parse", "HEAD")

	cli.git("checkout", "main")
	cli.git("merge", "--ff-only", "feature")

	repo := cli.open()
	head, err := repo.Head()
	require.NoError(t, err)
	require.Equal(t, featureHash, head.Hash().String())

	snap := snapshotHeadCommit(t, repo)
	require.Equal(t, "feature work", snap.Message)
}

func TestMergeFF_GoGitThenGit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)

	r.writeFile("a.txt", "a\n")
	r.git("add", "a.txt")
	r.git("commit", "-m", "base")

	r.git("checkout", "-b", "feature")
	r.writeFile("b.txt", "b\n")
	r.git("add", "b.txt")
	r.git("commit", "-m", "feature work")
	featureHash := r.git("rev-parse", "HEAD")

	r.git("checkout", "main")

	repo := r.open()
	featureRef, err := repo.Reference(plumbing.NewBranchReferenceName("feature"), true)
	require.NoError(t, err)
	err = repo.Merge(*featureRef, git.MergeOptions{
		Strategy: git.FastForwardMerge,
	})
	require.NoError(t, err)

	wt, err := repo.Worktree()
	require.NoError(t, err)
	err = wt.Reset(&git.ResetOptions{
		Commit: featureRef.Hash(),
		Mode:   git.HardReset,
	})
	require.NoError(t, err)

	r.fsck()
	require.Equal(t, featureHash, r.git("rev-parse", "HEAD"))
	require.Contains(t, r.git("log", "--oneline", "-1"), "feature work")
	require.FileExists(t, filepath.Join(r.dir, "b.txt"))
}
