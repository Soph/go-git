package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func cmdTag(args []string) int {
	var (
		doList    bool
		doDelete  bool
		annotated bool
		message   string
		positional []string
		nLines    int
	)

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-l", "--list":
			doList = true
		case "-d", "--delete":
			doDelete = true
		case "-a", "--annotate":
			annotated = true
		case "-m":
			if i+1 < len(args) {
				i++
				message = args[i]
				annotated = true
			}
		case "-n":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &nLines)
			}
		case "-f", "--force":
			// accepted, ignored
		default:
			if strings.HasPrefix(a, "-n") && len(a) > 2 {
				fmt.Sscanf(a[2:], "%d", &nLines)
			} else if strings.HasPrefix(a, "-m") && len(a) > 2 {
				message = a[2:]
				annotated = true
			} else if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
		i++
	}

	_ = nLines

	repo := openRepoOrDie()

	if doDelete {
		for _, name := range positional {
			err := repo.DeleteTag(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: tag '%s' not found.\n", name)
				return 1
			}
			fmt.Printf("Deleted tag '%s'\n", name)
		}
		return 0
	}

	if doList || len(positional) == 0 {
		iter, err := repo.Tags()
		if err != nil {
			return 0
		}
		var tags []string
		iter.ForEach(func(ref *plumbing.Reference) error {
			tags = append(tags, ref.Name().Short())
			return nil
		})
		sort.Strings(tags)
		for _, t := range tags {
			fmt.Println(t)
		}
		return 0
	}

	// Create tag.
	name := positional[0]
	var targetHash plumbing.Hash

	if len(positional) >= 2 {
		// Tag a specific object.
		h, err := repo.ResolveRevision(plumbing.Revision(positional[1]))
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: Failed to resolve '%s' as a valid ref.\n", positional[1])
			return 128
		}
		targetHash = *h
	} else {
		head, err := repo.Head()
		if err != nil {
			fmt.Fprintln(os.Stderr, "fatal: Failed to resolve 'HEAD' as a valid ref.")
			return 128
		}
		targetHash = head.Hash()
	}

	var opts *git.CreateTagOptions
	if annotated {
		sig := buildSignature("", "COMMITTER")
		if sig == nil {
			sig = &object.Signature{
				Name:  "unknown",
				Email: "unknown",
			}
		}
		opts = &git.CreateTagOptions{
			Tagger:  sig,
			Message: message,
		}
	}

	_, err := repo.CreateTag(name, targetHash, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	return 0
}
