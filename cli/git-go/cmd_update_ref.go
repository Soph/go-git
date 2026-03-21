package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
)

func cmdUpdateRef(args []string) int {
	var (
		doDelete   bool
		positional []string
	)

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-d", "--delete":
			doDelete = true
		case "--no-deref":
			// accepted, ignored
		case "-m":
			i++ // skip the reason message
		case "--stdin", "-z":
			// accepted, ignored
		default:
			if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
		i++
	}

	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "fatal: no refname specified")
		return 128
	}

	repo := openRepoOrDie()
	refName := plumbing.ReferenceName(positional[0])

	if doDelete {
		if err := repo.Storer.RemoveReference(refName); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		return 0
	}

	if len(positional) < 2 {
		fmt.Fprintln(os.Stderr, "fatal: no hash specified")
		return 128
	}

	hash := plumbing.NewHash(positional[1])
	ref := plumbing.NewHashReference(refName, hash)
	if err := repo.Storer.SetReference(ref); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}
	return 0
}
