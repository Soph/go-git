package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
)

func cmdHashObject(args []string) int {
	var (
		write    bool
		objType = "blob"
		useStdin bool
		files    []string
	)

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-w":
			write = true
		case "-t":
			if i+1 < len(args) {
				i++
				objType = args[i]
			}
		case "--stdin":
			useStdin = true
		default:
			if !strings.HasPrefix(a, "-") {
				files = append(files, a)
			}
		}
		i++
	}

	ot, err := plumbing.ParseObjectType(objType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: invalid object type '%s'\n", objType)
		return 128
	}

	// Open repo once if writing objects.
	var repo *git.Repository
	if write {
		repo = openRepoOrDie()
	}

	if useStdin {
		return hashFromReader(os.Stdin, ot, repo)
	}

	for _, f := range files {
		code := func() int {
			fh, err := os.Open(f)
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 128
			}
			defer fh.Close()
			return hashFromReader(fh, ot, repo)
		}()
		if code != 0 {
			return code
		}
	}
	return 0
}

func hashFromReader(r io.Reader, ot plumbing.ObjectType, repo *git.Repository) int {
	data, err := io.ReadAll(r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	if repo != nil {
		obj := repo.Storer.NewEncodedObject()
		obj.SetType(ot)
		obj.SetSize(int64(len(data)))
		w, err := obj.Writer()
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		if _, err := w.Write(data); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		if err := w.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		hash, err := repo.Storer.SetEncodedObject(obj)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		fmt.Println(hash)
	} else {
		hasher := plumbing.NewHasher(formatcfg.SHA1, ot, int64(len(data)))
		hasher.Write(data)
		fmt.Println(hasher.Sum())
	}
	return 0
}
