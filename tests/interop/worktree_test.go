//go:build interop

package interop_test

import (
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/stretchr/testify/require"
)

func TestStatus_Symmetric(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.seed()
	r.writeFile("committed.txt", "modified\n")
	r.writeFile("untracked.txt", "new\n")

	cliStatus := r.git("status", "--porcelain")
	require.Contains(t, cliStatus, "untracked.txt")

	ggStatus := snapshotStatus(t, r.open())
	require.NotEmpty(t, ggStatus, "go-git should report dirty status")
	require.Contains(t, ggStatus, "untracked.txt")
}

func TestCheckout_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)

	cli.writeFile("main.txt", "on main\n")
	cli.git("add", "main.txt")
	cli.git("commit", "-m", "main commit")

	cli.git("checkout", "-b", "feature")
	cli.writeFile("feature.txt", "on feature\n")
	cli.git("add", "feature.txt")
	cli.git("commit", "-m", "feature commit")

	cli.git("checkout", "main")

	repo := cli.open()
	head, err := repo.Head()
	require.NoError(t, err)
	require.Equal(t, plumbing.NewBranchReferenceName("main"), head.Name())

	snap := snapshotHeadCommit(t, repo)
	require.Equal(t, "main commit", snap.Message)
}

func TestCheckout_GoGitThenGit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)

	r.writeFile("main.txt", "on main\n")
	r.git("add", "main.txt")
	r.git("commit", "-m", "main commit")
	r.git("checkout", "-b", "feature")
	r.writeFile("feature.txt", "on feature\n")
	r.git("add", "feature.txt")
	r.git("commit", "-m", "feature commit")

	repo := r.open()
	wt, err := repo.Worktree()
	require.NoError(t, err)
	err = wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("main"),
	})
	require.NoError(t, err)

	r.fsck()
	branch := r.git("rev-parse", "--abbrev-ref", "HEAD")
	require.Equal(t, "main", branch)
	require.Contains(t, r.git("log", "--oneline", "-1"), "main commit")
	require.NoFileExists(t, filepath.Join(r.dir, "feature.txt"))
}

func TestHardReset_GoGitThenGit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)

	r.writeFile("a.txt", "first\n")
	r.git("add", "a.txt")
	r.git("commit", "-m", "first")
	firstHash := r.git("rev-parse", "HEAD")

	r.writeFile("b.txt", "second\n")
	r.git("add", "b.txt")
	r.git("commit", "-m", "second")

	repo := r.open()
	wt, err := repo.Worktree()
	require.NoError(t, err)
	err = wt.Reset(&git.ResetOptions{
		Commit: plumbing.NewHash(firstHash),
		Mode:   git.HardReset,
	})
	require.NoError(t, err)

	r.fsck()
	require.Equal(t, firstHash, r.git("rev-parse", "HEAD"))
	require.Contains(t, r.git("log", "--oneline", "-1"), "first")
	require.NoFileExists(t, filepath.Join(r.dir, "b.txt"))
}

func TestHardReset_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)

	cli.writeFile("a.txt", "first\n")
	cli.git("add", "a.txt")
	cli.git("commit", "-m", "first")
	firstHash := cli.git("rev-parse", "HEAD")

	cli.writeFile("b.txt", "second\n")
	cli.git("add", "b.txt")
	cli.git("commit", "-m", "second")
	cli.git("reset", "--hard", firstHash)

	repo := cli.open()
	snap := snapshotHeadCommit(t, repo)
	require.Equal(t, "first", snap.Message)
	require.Equal(t, firstHash, cli.git("rev-parse", "HEAD"))
}

func TestGitignore_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)

	cli.writeFile(".gitignore", "*.log\nbuild/\n")
	cli.writeFile("tracked.txt", "tracked\n")
	cli.writeFile("debug.log", "should be ignored\n")
	cli.writeFile("build/output.bin", "should be ignored\n")
	cli.git("add", ".")
	cli.git("commit", "-m", "with gitignore")

	repo := cli.open()
	snap := snapshotHeadCommit(t, repo)
	treeOut := cli.git("ls-tree", "-r", snap.Tree)
	require.Contains(t, treeOut, ".gitignore")
	require.Contains(t, treeOut, "tracked.txt")
	require.NotContains(t, treeOut, "debug.log")
	require.NotContains(t, treeOut, "build/")

	status := snapshotStatus(t, repo)
	_, hasLog := status["debug.log"]
	require.False(t, hasLog, "debug.log should not appear in status (ignored)")
}

func TestGitignore_GoGitThenGit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)

	r.writeFile(".gitignore", "*.log\nbuild/\n")
	r.writeFile("tracked.txt", "tracked\n")
	r.writeFile("debug.log", "should be ignored\n")
	r.writeFile("build/output.bin", "should be ignored\n")

	repo := r.open()
	wt, err := repo.Worktree()
	require.NoError(t, err)

	_, err = wt.Add(".")
	require.NoError(t, err)
	goGitCommit(t, wt, "with gitignore")

	r.fsck()
	treeOut := r.git("ls-tree", "-r", "HEAD")
	require.Contains(t, treeOut, ".gitignore")
	require.Contains(t, treeOut, "tracked.txt")
	require.NotContains(t, treeOut, "debug.log")
	require.NotContains(t, treeOut, "build/")
}
