package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

func cmdInit(args []string) int {
	var (
		bare   bool
		branch string
		path   string
	)

	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--bare":
			bare = true
		case a == "-b" && i+1 < len(args):
			i++
			branch = args[i]
		case strings.HasPrefix(a, "-b") && len(a) > 2:
			branch = a[2:]
		case strings.HasPrefix(a, "--initial-branch="):
			branch = strings.TrimPrefix(a, "--initial-branch=")
		case a == "--initial-branch" && i+1 < len(args):
			i++
			branch = args[i]
		case strings.HasPrefix(a, "--template="):
			// Accepted, ignored (go-git has no template support).
		case a == "--template" && i+1 < len(args):
			i++ // skip value
		case a == "--separate-git-dir":
			i++ // skip value
		case a == "-q" || a == "--quiet":
			// Accepted, ignored.
		case a == "--shared" || strings.HasPrefix(a, "--shared="):
			// Accepted, ignored.
		case !strings.HasPrefix(a, "-"):
			path = a
		default:
			// Unknown flag, ignore gracefully.
		}
		i++
	}

	// Reject GIT_WORK_TREE during init (matches real git behavior).
	if os.Getenv("GIT_WORK_TREE") != "" && !bare {
		fatal("GIT_WORK_TREE (or --work-tree=<directory>) not allowed without specifying GIT_DIR (or --git-dir=<directory>)")
	}

	// Default branch: honor GIT_TEST_DEFAULT_INITIAL_BRANCH_NAME if -b not given.
	if branch == "" {
		branch = os.Getenv("GIT_TEST_DEFAULT_INITIAL_BRANCH_NAME")
	}
	if branch == "" {
		branch = "main"
	}

	// Default path is current directory.
	if path == "" {
		wd, err := os.Getwd()
		if err != nil {
			fatal("%s", err)
		}
		path = wd
	}

	// Make path absolute.
	absPath, err := filepath.Abs(path)
	if err != nil {
		fatal("cannot resolve path '%s': %s", path, err)
	}
	path = absPath

	branchRef := plumbing.NewBranchReferenceName(branch)
	opts := []git.InitOption{git.WithDefaultBranch(branchRef)}

	dotGit := path
	if !bare {
		dotGit = filepath.Join(path, ".git")
	}

	_, err = git.PlainInit(path, bare, opts...)
	if err != nil {
		if errors.Is(err, git.ErrTargetDirNotEmpty) {
			fmt.Printf("Reinitialized existing Git repository in %s/\n", dotGit)
			return 0
		}
		fatal("%s", err)
	}

	fmt.Printf("Initialized empty Git repository in %s/\n", dotGit)
	return 0
}
