package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v6"
)

// openRepo opens the git repository at the current directory (or GIT_DIR).
func openRepo() (*git.Repository, error) {
	dir := os.Getenv("GIT_DIR")
	if dir == "" {
		dir = "."
	}
	return git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
}

// openRepoOrDie opens the repository or prints a fatal error and exits.
func openRepoOrDie() *git.Repository {
	r, err := openRepo()
	if err != nil {
		fatal("%s", err)
	}
	return r
}

// formatTime formats a time for git log/show output.
func formatTime(t time.Time) string {
	return t.Format("Mon Jan 2 15:04:05 2006 -0700")
}

// nextVal consumes the next argument as a flag value, advancing *i.
// Returns the value and true, or empty and false if no next arg exists.
func nextVal(args []string, i *int) (string, bool) {
	if *i+1 < len(args) {
		*i++
		return args[*i], true
	}
	return "", false
}

// firstLine returns the first line of s.
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// fatal prints a fatal error message to stderr and exits with code 128.
// Note: os.Exit skips deferred calls. This is acceptable for a CLI where
// defers are only closing file handles (reclaimed by the OS anyway).
// Do not add deferred writes (lockfiles, flush) that depend on running.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "fatal: "+format+"\n", args...)
	os.Exit(128)
}

// gitDir returns the .git directory path for the current working directory.
// In a bare repository (no .git subdirectory, but HEAD exists), returns ".".
// Note: this duplicates the walk in openRepo/DetectDotGit, but is needed by
// cmd_config.go which reads raw config files without opening a full repo.
func gitDir() string {
	dir := os.Getenv("GIT_DIR")
	if dir != "" {
		return dir
	}
	// Walk up to find .git
	wd, err := os.Getwd()
	if err != nil {
		return ".git"
	}
	for {
		candidate := filepath.Join(wd, ".git")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
		// Check for bare repo (HEAD file exists in current dir).
		if _, err := os.Stat(filepath.Join(wd, "HEAD")); err == nil {
			if _, err := os.Stat(filepath.Join(wd, "refs")); err == nil {
				return wd
			}
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return ".git"
}
