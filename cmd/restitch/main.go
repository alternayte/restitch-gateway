// Package main provides the entry point for the restitch gateway.
package main

import (
	"fmt"
	"os"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	if len(os.Args) < 2 || os.Args[1][0] == '-' {
		// No subcommand or starts with flag → default to "run"
		os.Exit(runCmd(os.Args[1:]))
	}

	switch os.Args[1] {
	case "run":
		os.Exit(runCmd(os.Args[2:]))
	case "check":
		os.Exit(checkCmd(os.Args[2:]))
	case "version":
		versionCmd()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		fmt.Fprintf(os.Stderr, "usage: restitch [run|check|version] [flags]\n")
		os.Exit(2)
	}
}
