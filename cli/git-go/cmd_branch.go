package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
)

func cmdBranch(args []string) int {
	var (
		doDelete    bool
		doMove      bool
		showCurrent bool
		positional  []string
	)

	for _, a := range args {
		switch a {
		case "-d", "--delete", "-D":
			doDelete = true
		case "-m", "--move", "-M":
			doMove = true
		case "--show-current":
			showCurrent = true
		case "-l", "--list":
			// default behavior
		case "-a", "--all":
			// show all (including remotes) — simplified
		case "-v", "--verbose":
			// accepted, ignored
		case "-q", "--quiet":
			// accepted, ignored
		case "--create-reflog":
			// accepted, ignored
		default:
			if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
	}

	repo := openRepoOrDie()

	if showCurrent {
		head, err := repo.Head()
		if err == nil && head.Name().IsBranch() {
			fmt.Println(head.Name().Short())
		}
		return 0
	}

	if doDelete {
		// Determine which branch HEAD points to (works even on orphan branches).
		head, _ := repo.Head()
		headRef, _ := repo.Storer.Reference(plumbing.HEAD)
		var currentBranch plumbing.ReferenceName
		if head != nil {
			currentBranch = head.Name()
		} else if headRef != nil && headRef.Type() == plumbing.SymbolicReference {
			currentBranch = headRef.Target()
		}
		for _, name := range positional {
			refName := plumbing.NewBranchReferenceName(name)
			if currentBranch == refName {
				fmt.Fprintf(os.Stderr, "error: cannot delete branch '%s' used by worktree at '%s'\n", name, ".")
				return 1
			}
			err := repo.Storer.RemoveReference(refName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: branch '%s' not found.\n", name)
				return 1
			}
			fmt.Printf("Deleted branch %s.\n", name)
		}
		return 0
	}

	if doMove {
		if len(positional) == 2 {
			oldName := positional[0]
			newName := positional[1]
			oldRef := plumbing.NewBranchReferenceName(oldName)
			newRef := plumbing.NewBranchReferenceName(newName)

			ref, err := repo.Storer.Reference(oldRef)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: branch '%s' not found.\n", oldName)
				return 1
			}
			newReference := plumbing.NewHashReference(newRef, ref.Hash())
			if err := repo.Storer.SetReference(newReference); err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 128
			}
			if err := repo.Storer.RemoveReference(oldRef); err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 128
			}

			// Update HEAD if it pointed to the old branch.
			head, _ := repo.Storer.Reference(plumbing.HEAD)
			if head != nil && head.Type() == plumbing.SymbolicReference && head.Target() == oldRef {
				if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, newRef)); err != nil {
					fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
					return 128
				}
			}
		} else if len(positional) == 1 {
			// Rename current branch.
			newName := positional[0]
			head, err := repo.Head()
			if err != nil {
				fmt.Fprintln(os.Stderr, "fatal: not on any branch")
				return 128
			}
			oldRef := head.Name()
			newRef := plumbing.NewBranchReferenceName(newName)

			newReference := plumbing.NewHashReference(newRef, head.Hash())
			if err := repo.Storer.SetReference(newReference); err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 128
			}
			if err := repo.Storer.RemoveReference(oldRef); err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 128
			}
			if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, newRef)); err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 128
			}
		}
		return 0
	}

	// Create branch.
	if len(positional) >= 1 {
		name := positional[0]
		head, err := repo.Head()
		if err != nil {
			fmt.Fprintln(os.Stderr, "fatal: not a valid object name: 'HEAD'")
			return 128
		}
		refName := plumbing.NewBranchReferenceName(name)

		// Check if branch already exists.
		if _, err := repo.Storer.Reference(refName); err == nil {
			fmt.Fprintf(os.Stderr, "fatal: a branch named '%s' already exists.\n", name)
			return 128
		}

		ref := plumbing.NewHashReference(refName, head.Hash())
		if err := repo.Storer.SetReference(ref); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		return 0
	}

	// List branches.
	head, _ := repo.Head()
	currentBranch := ""
	if head != nil && head.Name().IsBranch() {
		currentBranch = head.Name().Short()
	}

	iter, err := repo.Branches()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	var branches []string
	iter.ForEach(func(ref *plumbing.Reference) error {
		branches = append(branches, ref.Name().Short())
		return nil
	})
	sort.Strings(branches)

	for _, b := range branches {
		if b == currentBranch {
			fmt.Printf("* %s\n", b)
		} else {
			fmt.Printf("  %s\n", b)
		}
	}

	return 0
}
