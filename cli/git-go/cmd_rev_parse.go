package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
)

func cmdRevParse(args []string) int {
	repo := openRepoOrDie()

	for _, a := range args {
		switch {
		case a == "--git-dir":
			gd := gitDir()
			abs, err := filepath.Abs(gd)
			if err == nil {
				gd = abs
			}
			fmt.Println(gd)

		case a == "--show-toplevel":
			gd := gitDir()
			if filepath.Base(gd) == ".git" {
				fmt.Println(filepath.Dir(gd))
			} else {
				// bare repo or GIT_DIR set
				abs, _ := filepath.Abs(gd)
				fmt.Println(abs)
			}

		case a == "--is-bare-repository":
			cfg, err := repo.Config()
			if err != nil {
				fmt.Println("false")
			} else {
				fmt.Println(cfg.Core.IsBare)
			}

		case a == "--show-ref-format":
			fmt.Println("files")

		case a == "--is-inside-work-tree":
			cfg, err := repo.Config()
			if err != nil || cfg.Core.IsBare {
				fmt.Println("false")
			} else {
				fmt.Println("true")
			}

		case a == "--is-inside-git-dir":
			fmt.Println("false")

		case a == "--show-cdup":
			// Distance from cwd to toplevel
			gd := gitDir()
			var toplevel string
			if filepath.Base(gd) == ".git" {
				toplevel = filepath.Dir(gd)
			} else {
				toplevel, _ = filepath.Abs(gd)
			}
			wd, _ := os.Getwd()
			rel, err := filepath.Rel(wd, toplevel)
			if err != nil {
				fmt.Println()
			} else if rel == "." {
				fmt.Println()
			} else {
				fmt.Println(rel + "/")
			}

		case a == "--show-prefix":
			gd := gitDir()
			var toplevel string
			if filepath.Base(gd) == ".git" {
				toplevel = filepath.Dir(gd)
			} else {
				toplevel, _ = filepath.Abs(gd)
			}
			wd, _ := os.Getwd()
			rel, err := filepath.Rel(toplevel, wd)
			if err != nil || rel == "." {
				fmt.Println()
			} else {
				fmt.Println(rel + "/")
			}

		case a == "--absolute-git-dir":
			gd := gitDir()
			abs, _ := filepath.Abs(gd)
			fmt.Println(abs)

		case strings.HasPrefix(a, "--"):
			// Unknown flag — ignore or pass through
			continue

		default:
			// Resolve a revision to a hash.
			hash, err := repo.ResolveRevision(plumbing.Revision(a))
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: ambiguous argument '%s': unknown revision or path not in the working tree.\n", a)
				return 128
			}
			fmt.Println(hash.String())
		}
	}

	return 0
}
