//go:build interop

package interop_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig_GoGitThenGit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)

	repo := r.open()
	mustCreateRemote(t, repo, "upstream", "https://example.com/repo.git")

	out := r.git("remote", "-v")
	require.Contains(t, out, "upstream")
	require.Contains(t, out, "https://example.com/repo.git")
}

func TestConfig_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)
	cli.git("remote", "add", "upstream", "https://example.com/repo.git")

	remotes := snapshotRemotes(t, cli.open())
	require.Equal(t, map[string][]string{
		"upstream": {"https://example.com/repo.git"},
	}, remotes)
}
