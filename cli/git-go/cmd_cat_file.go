package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func cmdCatFile(args []string) int {
	var (
		showType bool
		showSize bool
		prettyPrint bool
		checkExist bool
		objectArg string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-t":
			showType = true
		case "-s":
			showSize = true
		case "-p":
			prettyPrint = true
		case "-e":
			checkExist = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				objectArg = args[i]
			}
		}
	}

	if objectArg == "" {
		fmt.Fprintln(os.Stderr, "fatal: no object specified")
		return 128
	}

	repo := openRepoOrDie()

	// Resolve the argument to a hash.
	hash, err := repo.ResolveRevision(plumbing.Revision(objectArg))
	if err != nil {
		// Try as a raw hash.
		h := plumbing.NewHash(objectArg)
		if h.IsZero() {
			fmt.Fprintf(os.Stderr, "fatal: Not a valid object name %s\n", objectArg)
			return 128
		}
		hash = &h
	}

	obj, err := repo.Storer.EncodedObject(plumbing.AnyObject, *hash)
	if err != nil {
		if checkExist {
			return 1
		}
		fmt.Fprintf(os.Stderr, "fatal: Not a valid object name %s\n", objectArg)
		return 128
	}

	if checkExist {
		return 0
	}

	if showType {
		fmt.Println(obj.Type().String())
		return 0
	}

	if showSize {
		fmt.Println(obj.Size())
		return 0
	}

	if prettyPrint {
		return catFilePretty(obj)
	}

	// Default: raw content
	reader, err := obj.Reader()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}
	defer reader.Close()
	io.Copy(os.Stdout, reader)
	return 0
}

func catFilePretty(obj plumbing.EncodedObject) int {
	switch obj.Type() {
	case plumbing.CommitObject:
		commit := &object.Commit{}
		if err := commit.Decode(obj); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		fmt.Printf("tree %s\n", commit.TreeHash)
		for _, p := range commit.ParentHashes {
			fmt.Printf("parent %s\n", p)
		}
		fmt.Printf("author %s\n", formatSignature(&commit.Author))
		fmt.Printf("committer %s\n", formatSignature(&commit.Committer))
		if commit.Signature != "" {
			fmt.Printf("gpgsig %s\n", strings.ReplaceAll(commit.Signature, "\n", "\n "))
		}
		fmt.Printf("\n%s", commit.Message)

	case plumbing.TreeObject:
		tree := &object.Tree{}
		if err := tree.Decode(obj); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		for _, e := range tree.Entries {
			fmt.Printf("%06o %s %s\t%s\n", uint32(e.Mode), modeToType(e.Mode), e.Hash, e.Name)
		}

	case plumbing.BlobObject:
		reader, err := obj.Reader()
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		defer reader.Close()
		io.Copy(os.Stdout, reader)

	case plumbing.TagObject:
		tag := &object.Tag{}
		if err := tag.Decode(obj); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		fmt.Printf("object %s\n", tag.Target)
		fmt.Printf("type %s\n", tag.TargetType.String())
		fmt.Printf("tag %s\n", tag.Name)
		fmt.Printf("tagger %s\n", formatSignature(&tag.Tagger))
		fmt.Printf("\n%s", tag.Message)

	default:
		reader, err := obj.Reader()
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		defer reader.Close()
		io.Copy(os.Stdout, reader)
	}

	return 0
}

func formatSignature(sig *object.Signature) string {
	return fmt.Sprintf("%s <%s> %d %s",
		sig.Name, sig.Email,
		sig.When.Unix(),
		sig.When.Format("-0700"),
	)
}

