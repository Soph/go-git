package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
)

func cmdFetch(args []string) int {
	var (
		remoteName string
		all        bool
		tags       bool
		noTags     bool
		prune      bool
		force      bool
		depth      int
		refSpecs   []string
	)

	positional := []string{}
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "--all":
			all = true
		case "--tags", "-t":
			tags = true
		case "--no-tags":
			noTags = true
		case "--prune", "-p":
			prune = true
		case "-f", "--force":
			force = true
		case "-q", "--quiet":
			// accepted, ignored
		case "--depth":
			if i+1 < len(args) {
				i++
				if _, err := fmt.Sscanf(args[i], "%d", &depth); err != nil {
					fmt.Fprintf(os.Stderr, "fatal: invalid depth: %s\n", args[i])
					return 128
				}
			}
		default:
			if v, ok := strings.CutPrefix(a, "--depth="); ok {
				if _, err := fmt.Sscanf(v, "%d", &depth); err != nil {
					fmt.Fprintf(os.Stderr, "fatal: invalid depth: %s\n", v)
					return 128
				}
			} else if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
		i++
	}

	if len(positional) >= 1 {
		remoteName = positional[0]
		refSpecs = positional[1:]
	}
	if remoteName == "" && !all {
		remoteName = "origin"
	}

	repo := openRepoOrDie()

	if all {
		remotes, err := repo.Remotes()
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		for _, r := range remotes {
			opts := &git.FetchOptions{
				RemoteName: r.Config().Name,
				Force:      force,
				Prune:      prune,
			}
			setFetchTags(opts, tags, noTags)
			err := repo.Fetch(opts)
			if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 128
			}
		}
		return 0
	}

	opts := &git.FetchOptions{
		RemoteName: remoteName,
		Force:      force,
		Prune:      prune,
		Depth:      depth,
	}
	setFetchTags(opts, tags, noTags)

	if len(refSpecs) > 0 {
		for _, rs := range refSpecs {
			if !strings.Contains(rs, ":") && !strings.HasPrefix(rs, "refs/") {
				rs = fmt.Sprintf("refs/heads/%s:refs/remotes/%s/%s", rs, remoteName, rs)
			}
			opts.RefSpecs = append(opts.RefSpecs, config.RefSpec(rs))
		}
	}

	err := repo.Fetch(opts)
	if err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	return 0
}

func setFetchTags(opts *git.FetchOptions, tags, noTags bool) {
	if noTags {
		opts.Tags = plumbing.NoTags
	} else if tags {
		opts.Tags = plumbing.AllTags
	}
}
