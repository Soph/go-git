package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
)

func cmdPush(args []string) int {
	var (
		remoteName string
		force      bool
		all        bool
		tags       bool
		prune      bool
		refSpecs   []string
	)

	positional := []string{}
	for _, a := range args {
		switch a {
		case "-f", "--force":
			force = true
		case "--all":
			all = true
		case "--tags":
			tags = true
		case "--prune":
			prune = true
		case "-u", "--set-upstream":
			// accepted, ignored (go-git doesn't track upstream)
		case "-q", "--quiet":
			// accepted, ignored
		case "--dry-run", "-n":
			// accepted, ignored
		case "--delete":
			// handled via refspec
		default:
			if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
	}

	// Parse positional: [remote] [refspec...]
	if len(positional) >= 1 {
		remoteName = positional[0]
		refSpecs = positional[1:]
	}
	if remoteName == "" {
		remoteName = "origin"
	}

	repo := openRepoOrDie()

	opts := &git.PushOptions{
		RemoteName: remoteName,
		Force:      force,
		Prune:      prune,
	}

	if all {
		opts.RefSpecs = []config.RefSpec{"refs/heads/*:refs/heads/*"}
	} else if tags {
		opts.RefSpecs = []config.RefSpec{"refs/tags/*:refs/tags/*"}
	} else if len(refSpecs) > 0 {
		for _, rs := range refSpecs {
			// Handle "branch" as "refs/heads/branch:refs/heads/branch".
			if !strings.Contains(rs, ":") && !strings.HasPrefix(rs, "refs/") {
				rs = fmt.Sprintf("refs/heads/%s:refs/heads/%s", rs, rs)
			}
			opts.RefSpecs = append(opts.RefSpecs, config.RefSpec(rs))
		}
	}

	err := repo.Push(opts)
	if err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			fmt.Fprintln(os.Stderr, "Everything up-to-date")
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: failed to push some refs to '%s'\n", remoteName)
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 1
	}

	return 0
}
