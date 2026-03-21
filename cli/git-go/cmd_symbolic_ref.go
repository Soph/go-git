package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
)

func cmdSymbolicRef(args []string) int {
	var (
		quiet   bool
		short   bool
		positional []string
	)

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-q", "--quiet":
			quiet = true
		case "--short":
			short = true
		case "-m":
			i++ // skip the reason message
		default:
			if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
		i++
	}

	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "fatal: no symbolic ref name specified")
		return 128
	}

	repo := openRepoOrDie()
	refName := plumbing.ReferenceName(positional[0])

	if len(positional) == 1 {
		// Read mode: print the target of the symbolic ref.
		ref, err := repo.Storer.Reference(refName)
		if err != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "fatal: ref %s is not a symbolic ref\n", refName)
			}
			return 1
		}
		if ref.Type() != plumbing.SymbolicReference {
			if !quiet {
				fmt.Fprintf(os.Stderr, "fatal: ref %s is not a symbolic ref\n", refName)
			}
			return 1
		}
		if short {
			fmt.Println(ref.Target().Short())
		} else {
			fmt.Println(ref.Target().String())
		}
		return 0
	}

	// Write mode: set the symbolic ref.
	target := plumbing.ReferenceName(positional[1])
	newRef := plumbing.NewSymbolicReference(refName, target)
	if err := repo.Storer.SetReference(newRef); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}
	return 0
}
