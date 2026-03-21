//go:build interop

// Package interop provides bidirectional compatibility tests between go-git
// and the git CLI. Each test performs an operation in one direction and
// validates the result from the other:
//
//   - git CLI -> go-git: perform an operation with the real git binary, then
//     open the repository with go-git and verify the result.
//   - go-git -> git CLI: perform the same operation with go-git, then shell
//     out to the real git binary and verify the result.
//
// These tests require the git binary on PATH and are gated behind the
// "interop" build tag so they don't run during normal `go test ./...`.
package interop_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommit_GitThenGoGit(t *testing.T) {
	t.Parallel()
	cli := newRepo(t)
	cli.writeFile("hello.txt", "hello world\n")
	cli.git("add", "hello.txt")
	cli.git("commit", "-m", "initial commit")

	repo := cli.open()
	snap := snapshotHeadCommit(t, repo)
	require.Equal(t, "initial commit", snap.Message)
	require.Equal(t, "Interop Test", snap.AuthorName)
	require.Equal(t, "interop@test.local", snap.AuthorEmail)
	require.Equal(t, 0, snap.ParentCount)
	require.Equal(t, cli.git("rev-parse", "HEAD^{tree}"), snap.Tree)
}

func TestCommit_GoGitThenGit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)

	repo := r.open()
	wt, err := repo.Worktree()
	require.NoError(t, err)

	r.writeFile("hello.txt", "hello world\n")
	_, err = wt.Add("hello.txt")
	require.NoError(t, err)

	hash := goGitCommit(t, wt, "initial commit")
	require.False(t, hash.IsZero())

	r.fsck()
	require.Equal(t, "initial commit", r.git("log", "-1", "--format=%s"))
	require.Equal(t, "Interop Test", r.git("log", "-1", "--format=%an"))
	require.Equal(t, "interop@test.local", r.git("log", "-1", "--format=%ae"))
	require.Equal(t, "", r.git("log", "-1", "--format=%P")) // no parents
	require.Contains(t, r.git("ls-tree", "-r", "HEAD"), "hello.txt")
}

func TestCommitChain_Symmetric(t *testing.T) {
	t.Parallel()

	files := []struct {
		name, content, msg string
	}{
		{"a.txt", "aaa\n", "add a"},
		{"b.txt", "bbb\n", "add b"},
		{"c.txt", "ccc\n", "add c"},
	}

	cli := newRepo(t)
	for _, f := range files {
		cli.writeFile(f.name, f.content)
		cli.git("add", f.name)
		cli.git("commit", "-m", f.msg)
	}

	gg := newRepo(t)
	ggRepo := gg.open()
	ggWt, err := ggRepo.Worktree()
	require.NoError(t, err)
	for _, f := range files {
		gg.writeFile(f.name, f.content)
		_, err := ggWt.Add(f.name)
		require.NoError(t, err)
		goGitCommit(t, ggWt, f.msg)
	}

	cli.fsck()
	gg.fsck()

	cliTree := cli.git("rev-parse", "HEAD^{tree}")
	ggHead, err := ggRepo.Head()
	require.NoError(t, err)
	ggCommit, err := ggRepo.CommitObject(ggHead.Hash())
	require.NoError(t, err)
	ggTree := ggCommit.TreeHash.String()

	require.Equal(t, cliTree, ggTree,
		"tree hash mismatch: CLI produced %s, go-git produced %s", cliTree, ggTree)
}

func TestCommitWithRemoval_Symmetric(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, useGoGit bool) (string, *repo) {
		t.Helper()
		r := newRepo(t)

		r.writeFile("keep.txt", "keep\n")
		r.writeFile("remove.txt", "remove\n")
		r.git("add", ".")
		r.git("commit", "-m", "initial")

		if useGoGit {
			repo := r.open()
			wt, err := repo.Worktree()
			require.NoError(t, err)

			require.NoError(t, os.Remove(filepath.Join(r.dir, "remove.txt")))
			_, err = wt.Remove("remove.txt")
			require.NoError(t, err)

			goGitCommit(t, wt, "remove file")
		} else {
			r.git("rm", "remove.txt")
			r.git("commit", "-m", "remove file")
		}

		r.fsck()
		return r.git("rev-parse", "HEAD^{tree}"), r
	}

	cliTree, _ := run(t, false)
	ggTree, ggRepo := run(t, true)

	require.Equal(t, cliTree, ggTree,
		"tree hash mismatch after file removal: CLI=%s go-git=%s", cliTree, ggTree)

	treeOut := ggRepo.git("ls-tree", "-r", "HEAD")
	require.Contains(t, treeOut, "keep.txt")
	require.NotContains(t, treeOut, "remove.txt")
}

func TestDeepPaths_Symmetric(t *testing.T) {
	t.Parallel()

	paths := []string{
		"a/b/c/deep.txt",
		"a/b/sibling.txt",
		"a/top.txt",
		"root.txt",
	}

	run := func(t *testing.T, useGoGit bool) (string, *repo) {
		t.Helper()
		r := newRepo(t)

		for i, p := range paths {
			r.writeFile(p, fmt.Sprintf("content-%d\n", i))
		}

		if useGoGit {
			repo := r.open()
			wt, err := repo.Worktree()
			require.NoError(t, err)
			for _, p := range paths {
				_, err = wt.Add(p)
				require.NoError(t, err)
			}
			goGitCommit(t, wt, "deep paths")
		} else {
			for _, p := range paths {
				r.git("add", p)
			}
			r.git("commit", "-m", "deep paths")
		}

		r.fsck()
		return r.git("rev-parse", "HEAD^{tree}"), r
	}

	cliTree, _ := run(t, false)
	ggTree, ggRepo := run(t, true)

	require.Equal(t, cliTree, ggTree,
		"tree hash mismatch for deep paths: CLI=%s go-git=%s", cliTree, ggTree)

	treeOut := ggRepo.git("ls-tree", "-r", "HEAD")
	for _, p := range paths {
		require.Contains(t, treeOut, p)
	}
}

func TestBinaryFile_Symmetric(t *testing.T) {
	t.Parallel()

	binaryContent := string([]byte{
		0x89, 0x50, 0x4E, 0x47,
		0x00, 0x00, 0x00, 0x0D,
		0xFF, 0xFE, 0xFD,
		0x0D, 0x0A, 0x0A, 0x0D,
		0x00, 0x01, 0x02, 0x03,
	})

	run := func(t *testing.T, useGoGit bool) string {
		t.Helper()
		r := newRepo(t)

		r.writeFile("image.bin", binaryContent)

		if useGoGit {
			repo := r.open()
			wt, err := repo.Worktree()
			require.NoError(t, err)
			_, err = wt.Add("image.bin")
			require.NoError(t, err)
			goGitCommit(t, wt, "add binary")
		} else {
			r.git("add", "image.bin")
			r.git("commit", "-m", "add binary")
		}

		r.fsck()
		return r.git("rev-parse", "HEAD^{tree}")
	}

	cliTree := run(t, false)
	ggTree := run(t, true)
	require.Equal(t, cliTree, ggTree,
		"tree hash mismatch for binary file: CLI=%s go-git=%s", cliTree, ggTree)
}

func TestBinaryFile_GoGitThenGit(t *testing.T) {
	t.Parallel()

	binaryContent := []byte{0x00, 0x01, 0xFF, 0xFE, 0x0D, 0x0A, 0x00, 0x80}

	r := newRepo(t)
	r.writeFile("data.bin", string(binaryContent))

	repo := r.open()
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("data.bin")
	require.NoError(t, err)
	goGitCommit(t, wt, "add binary data")

	r.fsck()
	require.Contains(t, r.git("ls-tree", "-r", "HEAD"), "data.bin")

	data, err := os.ReadFile(filepath.Join(r.dir, "data.bin"))
	require.NoError(t, err)
	require.Equal(t, binaryContent, data)
}
