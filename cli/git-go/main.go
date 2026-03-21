package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// command is a subcommand handler. It receives the remaining args after the
// subcommand name and returns an exit code.
type command func(args []string) int

var commands map[string]command

// allCommands returns the list of known subcommand names (for install symlinks).
func allCommands() []string {
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	return names
}

func init() {
	commands = map[string]command{
		"version":      cmdVersion,
		"init":         cmdInit,
		"config":       cmdConfig,
		"install":      cmdInstall,
		"hash-object":  cmdHashObject,
		"update-ref":   cmdUpdateRef,
		"symbolic-ref": cmdSymbolicRef,
		"show-ref":     cmdShowRef,
		"rev-parse":    cmdRevParse,
		"cat-file":     cmdCatFile,
		"ls-tree":      cmdLsTree,
		"add":          cmdAdd,
		"commit":       cmdCommit,
		"status":       cmdStatus,
		"branch":       cmdBranch,
		"tag":          cmdTag,
		"log":          cmdLog,
		"checkout":     cmdCheckout,
		"remote":       cmdRemote,
		"push":         cmdPush,
		"fetch":        cmdFetch,
		"diff":         cmdUnimplemented,
		"diff-files":   cmdUnimplemented,
		"diff-index":   cmdUnimplemented,
		"merge":        cmdUnimplemented,
		"reset":        cmdUnimplemented,
		"rm":           cmdUnimplemented,
		"clone":        cmdUnimplemented,
		"pull":         cmdUnimplemented,
		"for-each-ref": cmdUnimplemented,
	}
}

func main() {
	// Multicall: if invoked as "git-foo", treat "foo" as the subcommand.
	base := filepath.Base(os.Args[0])
	if strings.HasPrefix(base, "git-") {
		subcmd := strings.TrimPrefix(base, "git-")
		os.Exit(dispatch(subcmd, os.Args[1:]))
	}

	// Normal: "git [global-flags] <command> [args...]"
	globals, subcmd, args := parseGlobalFlags(os.Args[1:])

	if globals.showVersion {
		os.Exit(cmdVersion(nil))
	}
	if globals.showExecPath {
		fmt.Println(execPath())
		os.Exit(0)
	}

	if subcmd == "" {
		fmt.Fprintln(os.Stderr, "usage: git [--version] [--exec-path] [-C <path>] [-c <key>=<value>] <command> [<args>]")
		os.Exit(1)
	}

	applyGlobals(globals)
	// Prepend any forwarded global flags to the subcommand args.
	if len(globals.forwardArgs) > 0 {
		args = append(globals.forwardArgs, args...)
	}
	os.Exit(dispatch(subcmd, args))
}

func dispatch(name string, args []string) int {
	fn, ok := commands[name]
	if !ok {
		logUnimplemented(name)
		fmt.Fprintf(os.Stderr, "git: '%s' is not a git command. See 'git --help'.\n", name)
		return 1
	}
	return fn(args)
}

func logUnimplemented(cmd string) {
	logPath := os.Getenv("GIT_TEST_UNIMPLEMENTED_LOG")
	if logPath == "" {
		return
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\n", cmd)
}

func cmdUnimplemented(args []string) int {
	// Recover the command name from argv[0] or args.
	cmd := "unknown"
	base := filepath.Base(os.Args[0])
	if strings.HasPrefix(base, "git-") {
		cmd = strings.TrimPrefix(base, "git-")
	} else if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	logUnimplemented(cmd)
	fmt.Fprintf(os.Stderr, "fatal: command not yet implemented in go-git\n")
	return 128
}
