//go:build interop

package interop_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/stretchr/testify/require"
)

func TestPushFetch_GoGitToGit(t *testing.T) {
	t.Parallel()

	bare := newBareRepo(t)
	cliBare := newBareRepo(t)

	work := newRepo(t)
	work.writeFile("hello.txt", "hello\n")
	work.git("add", "hello.txt")
	work.git("commit", "-m", "first")

	ggRepo := work.open()
	mustCreateRemote(t, ggRepo, "origin", bare.dir)
	err := ggRepo.Push(&git.PushOptions{RemoteName: "origin"})
	require.NoError(t, err)

	cliWork := newRepo(t)
	cliWork.writeFile("hello.txt", "hello\n")
	cliWork.git("add", "hello.txt")
	cliWork.git("commit", "-m", "first")
	cliWork.git("remote", "add", "origin", cliBare.dir)
	cliWork.git("push", "origin", "main")

	bare.fsck()
	cliBare.fsck()

	// Both bare repos should have the same branch and commit via git CLI.
	require.Equal(t, cliBare.git("branch"), bare.git("branch"))
	require.Equal(t, cliBare.git("rev-parse", "main"), bare.git("rev-parse", "main"))
	require.Equal(t, cliBare.git("log", "-1", "--format=%s", "main"), bare.git("log", "-1", "--format=%s", "main"))
}

func TestPushFetch_GitToGoGit(t *testing.T) {
	t.Parallel()

	bare := newBareRepo(t)

	work := newRepo(t)
	work.writeFile("hello.txt", "hello\n")
	work.git("add", "hello.txt")
	work.git("commit", "-m", "first")
	work.git("remote", "add", "origin", bare.dir)
	work.git("push", "origin", "main")

	bareRepo := bare.openBare()
	require.Equal(t, map[string]string{"main": work.git("rev-parse", "HEAD")}, snapshotBranches(t, bareRepo))
	require.Equal(t, commitSnapshot{
		Tree:           work.git("rev-parse", "HEAD^{tree}"),
		Message:        "first",
		AuthorName:     "Interop Test",
		AuthorEmail:    "interop@test.local",
		CommitterName:  "Interop Test",
		CommitterEmail: "interop@test.local",
		ParentCount:    0,
	}, snapshotHeadCommit(t, bareRepo))
}

func TestClone_GitThenGoGit(t *testing.T) {
	t.Parallel()

	origin := newBareRepo(t)
	work := newRepo(t)
	work.writeFile("hello.txt", "hello\n")
	work.git("add", "hello.txt")
	work.git("commit", "-m", "initial")
	work.git("remote", "add", "origin", origin.dir)
	work.git("push", "origin", "main")

	cloneDir := tempDir(t, "interop-clone-*")
	repo, err := git.PlainClone(cloneDir, &git.CloneOptions{
		URL: origin.dir,
	})
	require.NoError(t, err)

	snap := snapshotHeadCommit(t, repo)
	require.Equal(t, "initial", snap.Message)

	data, err := os.ReadFile(filepath.Join(cloneDir, "hello.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello\n", string(data))
}

func TestClone_GoGitThenGit(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	ggRepo := r.open()
	wt, err := ggRepo.Worktree()
	require.NoError(t, err)
	r.writeFile("hello.txt", "hello\n")
	_, err = wt.Add("hello.txt")
	require.NoError(t, err)
	goGitCommit(t, wt, "initial")

	bare := newBareRepo(t)
	mustCreateRemote(t, ggRepo, "origin", bare.dir)
	err = ggRepo.Push(&git.PushOptions{RemoteName: "origin"})
	require.NoError(t, err)

	cloned := &repo{t: t, dir: tempDir(t, "interop-clone-cli-*")}
	cloned.git("clone", bare.dir, ".")

	cloned.fsck()
	require.Contains(t, cloned.git("log", "--oneline", "-1"), "initial")
	require.Equal(t, "hello\n", cloned.readFile("hello.txt"))
}
