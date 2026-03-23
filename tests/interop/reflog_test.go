//go:build interop

package interop_test

import (
	"strings"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/reflog"
	"github.com/stretchr/testify/require"
)

func TestReflog_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)

	cli.writeFile("a.txt", "aaa\n")
	cli.git("add", "a.txt")
	cli.git("commit", "-m", "first commit")
	hash1 := cli.git("rev-parse", "HEAD")

	cli.writeFile("b.txt", "bbb\n")
	cli.git("add", "b.txt")
	cli.git("commit", "-m", "second commit")
	hash2 := cli.git("rev-parse", "HEAD")

	repo := cli.open()

	// Both HEAD and refs/heads/main should track the same two commits.
	for _, ref := range []plumbing.ReferenceName{plumbing.HEAD, plumbing.NewBranchReferenceName("main")} {
		entries := snapshotReflog(t, repo, ref)
		require.Len(t, entries, 2, "ref=%s", ref)

		require.Equal(t, plumbing.ZeroHash.String(), entries[0].OldHash)
		require.Equal(t, hash1, entries[0].NewHash)
		require.Contains(t, entries[0].Message, "commit")

		require.Equal(t, hash1, entries[1].OldHash)
		require.Equal(t, hash2, entries[1].NewHash)
		require.Contains(t, entries[1].Message, "commit")
	}
}

func TestReflog_GoGitThenGit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)

	// Create two commits via git CLI so we have real hashes.
	r.writeFile("a.txt", "aaa\n")
	r.git("add", "a.txt")
	r.git("commit", "-m", "first")
	hash1 := r.git("rev-parse", "HEAD")

	r.writeFile("b.txt", "bbb\n")
	r.git("add", "b.txt")
	r.git("commit", "-m", "second")
	hash2 := r.git("rev-parse", "HEAD")

	repo := r.open()
	rs := mustReflogStorer(t, repo)

	branchRef := plumbing.NewBranchReferenceName("test-branch")
	err := repo.Storer.SetReference(plumbing.NewHashReference(branchRef, plumbing.NewHash(hash2)))
	require.NoError(t, err)

	sig := reflog.Signature{
		Name:  "Interop Test",
		Email: "interop@test.local",
		When:  defaultSignature().When,
	}

	err = rs.AppendReflog(branchRef, &reflog.Entry{
		OldHash:   plumbing.ZeroHash,
		NewHash:   plumbing.NewHash(hash1),
		Committer: sig,
		Message:   "branch: Created from main",
	})
	require.NoError(t, err)

	err = rs.AppendReflog(branchRef, &reflog.Entry{
		OldHash:   plumbing.NewHash(hash1),
		NewHash:   plumbing.NewHash(hash2),
		Committer: sig,
		Message:   "commit: second",
	})
	require.NoError(t, err)

	// git log -g shows newest first
	out := r.git("log", "-g", "--format=%gs", "refs/heads/test-branch")
	lines := strings.Split(out, "\n")
	require.Len(t, lines, 2)
	require.Equal(t, "commit: second", lines[0])
	require.Equal(t, "branch: Created from main", lines[1])

	// go-git returns oldest first
	entries := snapshotReflog(t, repo, branchRef)
	require.Len(t, entries, 2)
	require.Equal(t, plumbing.ZeroHash.String(), entries[0].OldHash)
	require.Equal(t, hash1, entries[0].NewHash)
	require.Equal(t, "branch: Created from main", entries[0].Message)
	require.Equal(t, hash1, entries[1].OldHash)
	require.Equal(t, hash2, entries[1].NewHash)
	require.Equal(t, "commit: second", entries[1].Message)
}

func TestReflog_BranchCreation_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)
	cli.seed()

	cli.git("checkout", "-b", "feature")
	cli.writeFile("feature.txt", "feature\n")
	cli.git("add", "feature.txt")
	cli.git("commit", "-m", "feature commit")

	repo := cli.open()

	// HEAD reflog should have entries for seed + checkout + feature commit.
	headEntries := snapshotReflog(t, repo, plumbing.HEAD)
	require.GreaterOrEqual(t, len(headEntries), 3)

	featureEntries := snapshotReflog(t, repo, plumbing.NewBranchReferenceName("feature"))
	require.NotEmpty(t, featureEntries)

	lastFeature := featureEntries[len(featureEntries)-1]
	require.Equal(t, cli.git("rev-parse", "feature"), lastFeature.NewHash)
	require.Contains(t, lastFeature.Message, "commit")
}

func TestReflog_ChainIntegrity_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)

	for i, name := range []string{"a.txt", "b.txt", "c.txt"} {
		cli.writeFile(name, strings.Repeat("x", i+1)+"\n")
		cli.git("add", name)
		cli.git("commit", "-m", "add "+name)
	}

	repo := cli.open()
	entries := snapshotReflog(t, repo, plumbing.HEAD)
	require.Len(t, entries, 3)

	// Each entry's NewHash should be the next entry's OldHash.
	for i := 1; i < len(entries); i++ {
		require.Equal(t, entries[i-1].NewHash, entries[i].OldHash,
			"entry %d OldHash should match entry %d NewHash", i, i-1)
	}
	require.Equal(t, plumbing.ZeroHash.String(), entries[0].OldHash)
}

func TestReflog_Amend_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)

	cli.writeFile("file.txt", "original\n")
	cli.git("add", "file.txt")
	cli.git("commit", "-m", "original commit")
	hash1 := cli.git("rev-parse", "HEAD")

	cli.writeFile("file.txt", "amended\n")
	cli.git("add", "file.txt")
	cli.git("commit", "--amend", "-m", "amended commit")
	hash2 := cli.git("rev-parse", "HEAD")
	require.NotEqual(t, hash1, hash2)

	repo := cli.open()
	entries := snapshotReflog(t, repo, plumbing.HEAD)
	require.GreaterOrEqual(t, len(entries), 2)

	last := entries[len(entries)-1]
	require.Equal(t, hash2, last.NewHash)
	require.Contains(t, last.Message, "amend")
}

func TestReflog_Reset_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)

	cli.writeFile("a.txt", "a\n")
	cli.git("add", "a.txt")
	cli.git("commit", "-m", "first")
	hash1 := cli.git("rev-parse", "HEAD")

	cli.writeFile("b.txt", "b\n")
	cli.git("add", "b.txt")
	cli.git("commit", "-m", "second")

	cli.git("reset", "--hard", hash1)

	repo := cli.open()
	entries := snapshotReflog(t, repo, plumbing.HEAD)
	require.GreaterOrEqual(t, len(entries), 3)

	last := entries[len(entries)-1]
	require.Equal(t, hash1, last.NewHash)
	require.Contains(t, last.Message, "reset")
}
