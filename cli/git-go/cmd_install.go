package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// cmdInstall creates git-* symlinks and the templates directory in the same
// directory as the binary. This makes --exec-path work and allows the test
// framework to find subcommands.
func cmdInstall(args []string) int {
	dir := execPath()
	exe := filepath.Join(dir, "git")

	// Create symlinks for all registered subcommands.
	for _, name := range allCommands() {
		if name == "install" {
			continue
		}
		link := filepath.Join(dir, "git-"+name)
		// Remove existing symlink if present.
		os.Remove(link)
		if err := os.Symlink(exe, link); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot create symlink %s: %s\n", link, err)
		}
	}

	// Create the templates directory that test-lib.sh expects.
	templatesDir := filepath.Join(dir, "templates", "blt")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot create templates dir: %s\n", err)
	}

	fmt.Printf("Installed git-* symlinks and templates in %s\n", dir)
	return 0
}
