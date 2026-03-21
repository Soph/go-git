package main

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v6/config"
)

func cmdRemote(args []string) int {
	var verbose bool

	if len(args) == 0 {
		return remoteList(false)
	}

	subcmd := ""
	positional := []string{}
	for _, a := range args {
		switch a {
		case "-v", "--verbose":
			verbose = true
		case "add", "remove", "rm", "rename", "set-url", "get-url", "show":
			if subcmd == "" {
				subcmd = a
			}
		default:
			positional = append(positional, a)
		}
	}

	switch subcmd {
	case "add":
		if len(positional) < 2 {
			fmt.Fprintln(os.Stderr, "usage: git remote add <name> <url>")
			return 1
		}
		return remoteAdd(positional[0], positional[1])
	case "remove", "rm":
		if len(positional) < 1 {
			fmt.Fprintln(os.Stderr, "usage: git remote remove <name>")
			return 1
		}
		return remoteRemove(positional[0])
	case "":
		return remoteList(verbose)
	default:
		fmt.Fprintf(os.Stderr, "fatal: command not yet implemented: remote %s\n", subcmd)
		return 128
	}
}

func remoteList(verbose bool) int {
	repo := openRepoOrDie()
	remotes, err := repo.Remotes()
	if err != nil {
		return 0
	}
	for _, r := range remotes {
		c := r.Config()
		if verbose {
			for _, url := range c.URLs {
				fmt.Printf("%s\t%s (fetch)\n", c.Name, url)
				fmt.Printf("%s\t%s (push)\n", c.Name, url)
			}
		} else {
			fmt.Println(c.Name)
		}
	}
	return 0
}

func remoteAdd(name, url string) int {
	repo := openRepoOrDie()
	_, err := repo.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		return 128
	}
	return 0
}

func remoteRemove(name string) int {
	repo := openRepoOrDie()
	err := repo.DeleteRemote(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: No such remote: '%s'\n", name)
		return 2
	}
	return 0
}
