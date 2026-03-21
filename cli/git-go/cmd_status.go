package main

import (
	"fmt"
	"os"
	"sort"

	goGit "github.com/go-git/go-git/v6"
)

func cmdStatus(args []string) int {
	var (
		porcelain bool
		short     bool
		branch    bool
	)

	for _, a := range args {
		switch a {
		case "--porcelain", "--porcelain=v1":
			porcelain = true
		case "-s", "--short":
			short = true
		case "-b", "--branch":
			branch = true
		case "-z":
			// NUL-terminated, ignored
		case "-u", "--untracked-files", "--untracked-files=all":
			// default behavior
		case "--untracked-files=no":
			// TODO: implement
		}
	}

	repo := openRepoOrDie()
	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	status, err := wt.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	if porcelain || short {
		if branch && porcelain {
			head, err := repo.Head()
			if err == nil && head.Name().IsBranch() {
				fmt.Printf("## %s\n", head.Name().Short())
			} else {
				fmt.Println("## HEAD (no branch)")
			}
		}
		printPorcelain(status)
		return 0
	}

	// Long format.
	head, _ := repo.Head()
	if head != nil && head.Name().IsBranch() {
		fmt.Printf("On branch %s\n", head.Name().Short())
	}

	if len(status) == 0 {
		fmt.Println("nothing to commit, working tree clean")
		return 0
	}

	printPorcelain(status)
	return 0
}

func printPorcelain(status goGit.Status) {
	// Sort paths for deterministic output.
	paths := make([]string, 0, len(status))
	for p := range status {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		s := status[p]
		staging := string(s.Staging)
		worktree := string(s.Worktree)
		if staging == " " && worktree == " " {
			continue
		}
		fmt.Printf("%s%s %s\n", staging, worktree, p)
	}
}
