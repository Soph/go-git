//go:build interop

package interop_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndex_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)
	cli.seed()
	cli.writeFile("staged.txt", "staged\n")
	cli.writeFile("dir/nested.txt", "nested\n")
	cli.git("add", "staged.txt", "dir/nested.txt")

	require.Equal(t, []string{"dir/nested.txt", "seed.txt", "staged.txt"},
		snapshotIndex(t, cli.open()))
}

func TestIndex_GoGitThenGit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)

	r.seed()

	repo := r.open()
	wt, err := repo.Worktree()
	require.NoError(t, err)

	r.writeFile("staged.txt", "staged\n")
	r.writeFile("dir/nested.txt", "nested\n")
	_, err = wt.Add("staged.txt")
	require.NoError(t, err)
	_, err = wt.Add("dir/nested.txt")
	require.NoError(t, err)

	out := r.git("ls-files")
	require.Contains(t, out, "staged.txt")
	require.Contains(t, out, "dir/nested.txt")
	require.Contains(t, out, "seed.txt")
}
