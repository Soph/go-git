package main

import (
	"fmt"
	"runtime"
)

const goGitVersion = "2.47.0.go-git.1"

func cmdVersion(args []string) int {
	buildOptions := false
	for _, a := range args {
		if a == "--build-options" {
			buildOptions = true
		}
	}

	fmt.Printf("git version %s\n", goGitVersion)

	if buildOptions {
		fmt.Printf("cpu: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("sizeof-long: 8\n")
		fmt.Printf("sizeof-size_t: 8\n")
		fmt.Printf("shell-path: /bin/sh\n")
		fmt.Printf("default-ref-format: files\n")
		fmt.Printf("default-hash: sha1\n")
	}

	return 0
}
