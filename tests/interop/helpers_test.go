//go:build interop

package interop_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/filesystem"
	"github.com/stretchr/testify/require"
)

// repo is a test helper that wraps a temporary directory containing a git
// repository. It provides methods for both git CLI and go-git operations.
type repo struct {
	t   *testing.T
	dir string
}

// tempDir creates a temporary directory with symlinks resolved and registers cleanup.
func tempDir(t *testing.T, prefix string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", prefix)
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	// Resolve symlinks (macOS: /var -> /private/var).
	if resolved, e := filepath.EvalSymlinks(dir); e == nil {
		dir = resolved
	}
	return dir
}

// newRepo creates a fresh, isolated git repository in a temporary directory.
// GIT_CONFIG_GLOBAL is set to the platform null device so the user's global
// config cannot leak into the test.
func newRepo(t *testing.T) *repo {
	t.Helper()
	r := &repo{t: t, dir: tempDir(t, "interop-*")}
	r.git("init", "-b", "main")
	r.git("config", "user.name", "Interop Test")
	r.git("config", "user.email", "interop@test.local")
	return r
}

// git runs a git CLI command in the repo directory and returns its trimmed
// stdout. It fails the test on non-zero exit.
func (r *repo) git(args ...string) string {
	r.t.Helper()
	return r.gitWithEnv(nil, args...)
}

// gitWithEnv runs a git CLI command with additional environment variables.
func (r *repo) gitWithEnv(extraEnv []string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_DATE=2025-01-15T12:00:00+00:00",
		"GIT_COMMITTER_DATE=2025-01-15T12:00:00+00:00",
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	require.NoError(r.t, err, "git %s failed:\n%s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

func (r *repo) open() *git.Repository {
	r.t.Helper()
	dotgit := osfs.New(filepath.Join(r.dir, ".git"))
	wt := osfs.New(r.dir)
	st := filesystem.NewStorage(dotgit, cache.NewObjectLRUDefault())
	repo, err := git.Open(st, wt)
	require.NoError(r.t, err)
	return repo
}

func (r *repo) openBare() *git.Repository {
	r.t.Helper()
	dotgit := osfs.New(r.dir)
	st := filesystem.NewStorage(dotgit, cache.NewObjectLRUDefault())
	repo, err := git.Open(st, nil)
	require.NoError(r.t, err)
	return repo
}

// seed creates a single-file commit so the repo has a non-empty HEAD.
func (r *repo) seed() {
	r.t.Helper()
	r.writeFile("seed.txt", "seed\n")
	r.git("add", "seed.txt")
	r.git("commit", "-m", "seed")
}

func (r *repo) writeFile(name, content string) {
	r.t.Helper()
	path := filepath.Join(r.dir, name)
	require.NoError(r.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(r.t, os.WriteFile(path, []byte(content), 0o644))
}

func (r *repo) readFile(name string) string {
	r.t.Helper()
	data, err := os.ReadFile(filepath.Join(r.dir, name))
	require.NoError(r.t, err)
	return string(data)
}

func (r *repo) fsck() {
	r.t.Helper()
	r.git("fsck", "--strict")
}

// defaultSignature returns a consistent signature for go-git commits.
// The fixed timestamp makes commit hashes deterministic, which is useful for
// debugging but is not required for the symmetric tree-hash comparisons
// (tree hashes don't include commit timestamps).
func defaultSignature() *object.Signature {
	return &object.Signature{
		Name:  "Interop Test",
		Email: "interop@test.local",
		When:  time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
	}
}

// goGitCommit creates a commit using go-git with the default signature.
func goGitCommit(t *testing.T, wt *git.Worktree, msg string) plumbing.Hash {
	t.Helper()
	sig := defaultSignature()
	hash, err := wt.Commit(msg, &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	})
	require.NoError(t, err)
	return hash
}

type commitSnapshot struct {
	Tree           string
	Message        string
	AuthorName     string
	AuthorEmail    string
	CommitterName  string
	CommitterEmail string
	ParentCount    int
}

func snapshotHeadCommit(t *testing.T, repo *git.Repository) commitSnapshot {
	t.Helper()

	head, err := repo.Head()
	require.NoError(t, err)

	commit, err := repo.CommitObject(head.Hash())
	require.NoError(t, err)

	return commitSnapshot{
		Tree:           commit.TreeHash.String(),
		Message:        strings.TrimSuffix(commit.Message, "\n"),
		AuthorName:     commit.Author.Name,
		AuthorEmail:    commit.Author.Email,
		CommitterName:  commit.Committer.Name,
		CommitterEmail: commit.Committer.Email,
		ParentCount:    commit.NumParents(),
	}
}

func snapshotBranches(t *testing.T, repo *git.Repository) map[string]string {
	t.Helper()

	iter, err := repo.Branches()
	require.NoError(t, err)

	branches := map[string]string{}
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		branches[ref.Name().Short()] = ref.Hash().String()
		return nil
	})
	require.NoError(t, err)

	return branches
}

type tagSnapshot struct {
	Kind       string
	TargetHash string
	Name       string
	Message    string
}

func snapshotTag(t *testing.T, repo *git.Repository, name string) tagSnapshot {
	t.Helper()

	ref, err := repo.Tag(name)
	require.NoError(t, err)

	snapshot := tagSnapshot{}

	tagObj, err := repo.TagObject(ref.Hash())
	if err == nil {
		snapshot.Kind = "annotated"
		snapshot.TargetHash = tagObj.Target.String()
		snapshot.Name = tagObj.Name
		snapshot.Message = strings.TrimSuffix(tagObj.Message, "\n")
		return snapshot
	}

	require.True(t, errors.Is(err, plumbing.ErrObjectNotFound))
	snapshot.Kind = "lightweight"
	snapshot.TargetHash = ref.Hash().String()
	snapshot.Name = name
	return snapshot
}

func snapshotRemotes(t *testing.T, repo *git.Repository) map[string][]string {
	t.Helper()

	cfg, err := repo.Config()
	require.NoError(t, err)

	remotes := make(map[string][]string, len(cfg.Remotes))
	for name, remote := range cfg.Remotes {
		urls := append([]string(nil), remote.URLs...)
		sort.Strings(urls)
		remotes[name] = urls
	}

	return remotes
}

func snapshotIndex(t *testing.T, repo *git.Repository) []string {
	t.Helper()

	idx, err := repo.Storer.Index()
	require.NoError(t, err)

	names := make([]string, 0, len(idx.Entries))
	for _, e := range idx.Entries {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	return names
}

func snapshotStatus(t *testing.T, repo *git.Repository) map[string]string {
	t.Helper()

	wt, err := repo.Worktree()
	require.NoError(t, err)

	status, err := wt.StatusWithOptions(git.StatusOptions{Strategy: git.Preload})
	require.NoError(t, err)

	out := map[string]string{}
	for path, file := range status {
		if file.Staging == git.Unmodified && file.Worktree == git.Unmodified {
			continue
		}
		out[path] = string([]byte{byte(file.Staging), byte(file.Worktree)})
	}

	return out
}

// newBareRepo creates a fresh bare git repository in a temporary directory.
func newBareRepo(t *testing.T) *repo {
	t.Helper()
	bare := &repo{t: t, dir: tempDir(t, "interop-bare-*")}
	bare.git("init", "--bare", "-b", "main")
	return bare
}

func mustCreateRemote(t *testing.T, repo *git.Repository, name string, urls ...string) {
	t.Helper()

	_, err := repo.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: urls,
	})
	require.NoError(t, err)
}
