//go:build interop

package interop_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubmodule_GitThenGoGit(t *testing.T) {
	t.Parallel()

	lib := newRepo(t)
	lib.writeFile("lib.txt", "library code\n")
	lib.git("add", "lib.txt")
	lib.git("commit", "-m", "lib initial")

	parent := newRepo(t)
	parent.writeFile("main.txt", "main code\n")
	parent.git("add", "main.txt")
	parent.git("commit", "-m", "parent initial")
	parent.gitWithEnv([]string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=protocol.file.allow", "GIT_CONFIG_VALUE_0=always"},
		"submodule", "add", lib.dir, "vendor/lib")
	parent.git("commit", "-m", "add submodule")

	repo := parent.open()
	wt, err := repo.Worktree()
	require.NoError(t, err)

	subs, err := wt.Submodules()
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, "vendor/lib", subs[0].Config().Path)

	snap := snapshotHeadCommit(t, repo)
	treeOut := parent.git("ls-tree", "-r", snap.Tree)
	require.Contains(t, treeOut, ".gitmodules")
	require.Contains(t, treeOut, "vendor/lib")
}

func TestSubmodule_StatusReadback(t *testing.T) {
	t.Parallel()

	lib := newRepo(t)
	lib.writeFile("lib.txt", "library code\n")
	lib.git("add", "lib.txt")
	lib.git("commit", "-m", "lib initial")

	parent := newRepo(t)
	parent.writeFile("main.txt", "main code\n")
	parent.git("add", "main.txt")
	parent.git("commit", "-m", "parent initial")
	parent.gitWithEnv([]string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=protocol.file.allow", "GIT_CONFIG_VALUE_0=always"},
		"submodule", "add", lib.dir, "vendor/lib")
	parent.git("commit", "-m", "add submodule")

	parent.fsck()
	repo := parent.open()
	wt, err := repo.Worktree()
	require.NoError(t, err)
	subs, err := wt.Submodules()
	require.NoError(t, err)
	require.Len(t, subs, 1)

	statuses, err := subs.Status()
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.Equal(t, "vendor/lib", statuses[0].Path)
}
