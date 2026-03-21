package main

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v6"
)

func cmdAdd(args []string) int {
	var (
		all   bool
		paths []string
	)

	for _, a := range args {
		switch a {
		case "-A", "--all":
			all = true
		case "-u", "--update":
			all = true
		case "-n", "--dry-run":
			// accepted, ignored
		case "-v", "--verbose":
			// accepted, ignored
		case "-f", "--force":
			// accepted, ignored
		default:
			paths = append(paths, a)
		}
	}

	repo := openRepoOrDie()
	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	if all || (len(paths) == 1 && paths[0] == ".") {
		err = wt.AddWithOptions(&git.AddOptions{All: true})
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		return 0
	}

	for _, p := range paths {
		_, err := wt.Add(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: pathspec '%s' did not match any files\n", p)
			return 128
		}
	}
	return 0
}
