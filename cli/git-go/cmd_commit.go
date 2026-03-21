package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func cmdCommit(args []string) int {
	var (
		message      string
		all          bool
		allowEmpty   bool
		authorStr    string
		amend        bool
	)

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-m":
			if i+1 < len(args) {
				i++
				if message != "" {
					message += "\n\n"
				}
				message += args[i]
			}
		case "-a", "--all":
			all = true
		case "--allow-empty":
			allowEmpty = true
		case "--amend":
			amend = true
		case "--author":
			if i+1 < len(args) {
				i++
				authorStr = args[i]
			}
		case "-F":
			if i+1 < len(args) {
				i++
				data, err := os.ReadFile(args[i])
				if err != nil {
					fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
					return 128
				}
				message = string(data)
			}
		case "--no-edit":
			// accepted, ignored (for amend)
		case "-q", "--quiet":
			// accepted, ignored
		case "--cleanup=verbatim", "--cleanup=strip", "--cleanup=whitespace":
			// accepted, ignored
		default:
			if strings.HasPrefix(a, "-m") {
				message = a[2:]
			} else if strings.HasPrefix(a, "--author=") {
				authorStr = strings.TrimPrefix(a, "--author=")
			} else if strings.HasPrefix(a, "--cleanup=") {
				// accepted, ignored
			} else if strings.HasPrefix(a, "-F") {
				data, err := os.ReadFile(a[2:])
				if err != nil {
					fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
					return 128
				}
				message = string(data)
			}
		}
		i++
	}

	repo := openRepoOrDie()

	// For --amend --no-edit (or just --amend without -m), reuse the previous message.
	if message == "" && amend {
		head, err := repo.Head()
		if err != nil {
			fmt.Fprintln(os.Stderr, "fatal: cannot amend: no previous commit")
			return 128
		}
		prev, err := repo.CommitObject(head.Hash())
		if err != nil {
			fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
			return 128
		}
		message = prev.Message
	}

	if message == "" {
		fmt.Fprintln(os.Stderr, "error: switch `m' requires a value")
		return 128
	}
	wt, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	author := buildSignature(authorStr, "AUTHOR")
	committer := buildSignature("", "COMMITTER")

	opts := &git.CommitOptions{
		All:               all,
		AllowEmptyCommits: allowEmpty,
		Author:            author,
		Committer:         committer,
		Amend:             amend,
	}

	hash, err := wt.Commit(message, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	// Get branch name for output.
	head, _ := repo.Head()
	branch := "detached HEAD"
	if head != nil && head.Name().IsBranch() {
		branch = head.Name().Short()
	}

	commit, _ := repo.CommitObject(hash)
	subject := firstLine(message)

	isRoot := commit != nil && len(commit.ParentHashes) == 0
	if isRoot {
		fmt.Printf("[%s (root-commit) %s] %s\n", branch, hash.String()[:7], subject)
	} else {
		fmt.Printf("[%s %s] %s\n", branch, hash.String()[:7], subject)
	}

	return 0
}

// buildSignature creates a Signature from env vars or --author string.
func buildSignature(authorStr, role string) *object.Signature {
	name := os.Getenv("GIT_" + role + "_NAME")
	email := os.Getenv("GIT_" + role + "_EMAIL")
	dateStr := os.Getenv("GIT_" + role + "_DATE")

	if authorStr != "" && role == "AUTHOR" {
		// Parse "Name <email>" format.
		if idx := strings.Index(authorStr, " <"); idx >= 0 {
			name = authorStr[:idx]
			rest := authorStr[idx+2:]
			if end := strings.Index(rest, ">"); end >= 0 {
				email = rest[:end]
			}
		}
	}

	if name == "" || email == "" {
		return nil // let go-git read from config
	}

	when := time.Now()
	if dateStr != "" {
		// Try unix timestamp format: "1234567890 +0000"
		if parsed, err := parseGitDate(dateStr); err == nil {
			when = parsed
		}
	}

	return &object.Signature{
		Name:  name,
		Email: email,
		When:  when,
	}
}

func parseGitDate(s string) (time.Time, error) {
	// Try "@<unix> <tz>" format (used by test framework).
	if strings.HasPrefix(s, "@") {
		s = s[1:]
	}
	// Try "unix tz" format.
	parts := strings.Fields(s)
	if len(parts) >= 1 {
		var unix int64
		if _, err := fmt.Sscanf(parts[0], "%d", &unix); err == nil {
			t := time.Unix(unix, 0)
			if len(parts) >= 2 {
				loc, err := time.Parse("-0700", parts[1])
				if err == nil {
					t = t.In(loc.Location())
				}
			}
			return t, nil
		}
	}
	// Try RFC2822-ish.
	for _, layout := range []string{
		time.RFC1123Z,
		time.RFC3339,
		"Mon, 2 Jan 2006 15:04:05 -0700",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse date: %s", s)
}
