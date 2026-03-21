package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type globalFlags struct {
	dir             string            // -C <dir>
	configOverrides map[string]string // -c key=value
	showVersion     bool              // --version
	showExecPath    bool              // --exec-path (query mode)
	forwardArgs     []string          // flags to forward to the subcommand (e.g., --bare)
}

// parseGlobalFlags extracts global flags from args, returning the remaining
// subcommand name and its arguments.
func parseGlobalFlags(args []string) (globalFlags, string, []string) {
	g := globalFlags{
		configOverrides: make(map[string]string),
	}

	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "-C" && i+1 < len(args):
			i++
			g.dir = args[i]
		case strings.HasPrefix(a, "-C"):
			g.dir = a[2:]

		case a == "-c" && i+1 < len(args):
			i++
			if k, v, ok := strings.Cut(args[i], "="); ok {
				g.configOverrides[k] = v
			}
		case strings.HasPrefix(a, "-c") && strings.Contains(a, "="):
			rest := a[2:]
			if k, v, ok := strings.Cut(rest, "="); ok {
				g.configOverrides[k] = v
			}

		case a == "--version":
			g.showVersion = true
			return g, "", nil
		case a == "--exec-path":
			g.showExecPath = true
			return g, "", nil
		case strings.HasPrefix(a, "--exec-path="):
			// Set exec path (rarely used, ignore value for now).
		case a == "--bare":
			// --bare is a global flag that gets forwarded to the subcommand.
			// e.g., "git --bare init" → "init --bare"
			g.forwardArgs = append(g.forwardArgs, a)
		case a == "--":
			i++
			if i < len(args) {
				return g, args[i], args[i+1:]
			}
			return g, "", nil
		default:
			// First non-flag is the subcommand.
			return g, a, args[i+1:]
		}
		i++
	}

	return g, "", nil
}

// applyGlobals processes global flags that affect the environment.
func applyGlobals(g globalFlags) {
	if g.dir != "" {
		if err := os.Chdir(g.dir); err != nil {
			fmt.Fprintf(os.Stderr, "fatal: cannot change to '%s': %s\n", g.dir, err)
			os.Exit(128)
		}
	}
	// Config overrides are stored and applied when opening a repo.
	if len(g.configOverrides) > 0 {
		globalConfigOverrides = g.configOverrides
	}
}

// globalConfigOverrides holds -c key=value pairs for the current invocation.
var globalConfigOverrides map[string]string

// execPath returns the directory containing our binary (where git-* symlinks live).
func execPath() string {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: cannot determine executable path: %s\n", err)
		os.Exit(128)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}
