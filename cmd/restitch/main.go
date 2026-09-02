// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	case "import":
		os.Exit(importCmd(os.Args[2:]))
	case "dev":
		os.Exit(devCmd(os.Args[2:]))
	case "healthcheck":
		os.Exit(healthcheckCmd(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		fmt.Fprintf(os.Stderr, "usage: restitch [run|check|version|import|dev|healthcheck] [flags]\n")
		os.Exit(2)
	}
}
