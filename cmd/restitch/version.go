package main

import (
	"fmt"
	"runtime"
)

func versionCmd() {
	fmt.Printf("restitch %s (%s)\n", version, runtime.Version())
}
