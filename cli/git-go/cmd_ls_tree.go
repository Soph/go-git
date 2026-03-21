package main

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func cmdLsTree(args []string) int {
	var (
		recursive bool
		nameOnly  bool
		treeArg   string
	)

	for _, a := range args {
		switch a {
		case "-r":
			recursive = true
		case "--name-only", "--name-status":
			nameOnly = true
		case "-t":
			// show tree entries even when going recursive
		case "-d":
			// only show trees (not implemented fully, ignore)
		case "-z":
			// NUL-terminated (ignore for now)
		default:
			if treeArg == "" {
				treeArg = a
			}
		}
	}

	if treeArg == "" {
		fmt.Fprintln(os.Stderr, "fatal: not enough arguments")
		return 128
	}

	repo := openRepoOrDie()

	hash, err := repo.ResolveRevision(plumbing.Revision(treeArg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: Not a valid object name %s\n", treeArg)
		return 128
	}

	// Resolve to tree: could be a commit or tree hash.
	obj, err := repo.Storer.EncodedObject(plumbing.AnyObject, *hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: Not a valid object name %s\n", treeArg)
		return 128
	}

	var tree *object.Tree
	switch obj.Type() {
	case plumbing.CommitObject:
		commit, err := repo.CommitObject(*hash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		tree, err = commit.Tree()
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
	case plumbing.TreeObject:
		tree, err = object.GetTree(repo.Storer, *hash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
	default:
		fmt.Fprintf(os.Stderr, "fatal: not a tree object: %s\n", treeArg)
		return 128
	}

	if recursive {
		printTreeRecursive(tree, "", nameOnly)
	} else {
		printTreeEntries(tree.Entries, nameOnly)
	}
	return 0
}

func printTreeEntries(entries []object.TreeEntry, nameOnly bool) {
	for _, e := range entries {
		if nameOnly {
			fmt.Println(e.Name)
		} else {
			objType := modeToType(e.Mode)
			fmt.Printf("%06o %s %s\t%s\n", uint32(e.Mode), objType, e.Hash, e.Name)
		}
	}
}

func printTreeRecursive(tree *object.Tree, prefix string, nameOnly bool) {
	for _, e := range tree.Entries {
		name := e.Name
		if prefix != "" {
			name = prefix + "/" + name
		}
		if e.Mode == filemode.Dir {
			subtree, err := tree.Tree(e.Name)
			if err != nil {
				continue
			}
			printTreeRecursive(subtree, name, nameOnly)
		} else {
			if nameOnly {
				fmt.Println(name)
			} else {
				objType := modeToType(e.Mode)
				fmt.Printf("%06o %s %s\t%s\n", uint32(e.Mode), objType, e.Hash, name)
			}
		}
	}
}

func modeToType(m filemode.FileMode) string {
	switch m {
	case filemode.Dir:
		return "tree"
	case filemode.Submodule:
		return "commit"
	default:
		return "blob"
	}
}
