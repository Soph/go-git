package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func cmdLog(args []string) int {
	var (
		oneline bool
		maxCount int = -1
		format  string
	)

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--oneline":
			oneline = true
		case a == "-1":
			maxCount = 1
		case strings.HasPrefix(a, "-") && len(a) > 1 && a[1] >= '0' && a[1] <= '9':
			if _, err := fmt.Sscanf(a[1:], "%d", &maxCount); err != nil {
				fmt.Fprintf(os.Stderr, "fatal: invalid count: %s\n", a)
				return 128
			}
		case a == "-n" && i+1 < len(args):
			i++
			if _, err := fmt.Sscanf(args[i], "%d", &maxCount); err != nil {
				fmt.Fprintf(os.Stderr, "fatal: invalid count: %s\n", args[i])
				return 128
			}
		case strings.HasPrefix(a, "--format="):
			format = strings.TrimPrefix(a, "--format=")
		case strings.HasPrefix(a, "--pretty="):
			format = strings.TrimPrefix(a, "--pretty=")
			if format == "oneline" {
				oneline = true
				format = ""
			}
		}
	}

	repo := openRepoOrDie()

	logOpts := &git.LogOptions{}
	iter, err := repo.Log(logOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}

	count := 0
	iter.ForEach(func(c *object.Commit) error {
		if maxCount >= 0 && count >= maxCount {
			return fmt.Errorf("stop")
		}

		if format != "" {
			line := expandFormat(format, c)
			fmt.Println(line)
		} else if oneline {
			fmt.Printf("%s %s\n", c.Hash.String()[:7], firstLine(c.Message))
		} else {
			fmt.Printf("commit %s\n", c.Hash)
			fmt.Printf("Author: %s <%s>\n", c.Author.Name, c.Author.Email)
			fmt.Printf("Date:   %s\n", formatTime(c.Author.When))
			fmt.Println()
			// Indent message.
			for _, line := range strings.Split(strings.TrimRight(c.Message, "\n"), "\n") {
				fmt.Printf("    %s\n", line)
			}
			fmt.Println()
		}
		count++
		return nil
	})

	return 0
}

func expandFormat(format string, c *object.Commit) string {
	r := strings.NewReplacer(
		"%H", c.Hash.String(),
		"%h", c.Hash.String()[:7],
		"%s", firstLine(c.Message),
		"%B", strings.TrimRight(c.Message, "\n"),
		"%an", c.Author.Name,
		"%ae", c.Author.Email,
		"%cn", c.Committer.Name,
		"%ce", c.Committer.Email,
		"%n", "\n",
	)
	return r.Replace(format)
}
