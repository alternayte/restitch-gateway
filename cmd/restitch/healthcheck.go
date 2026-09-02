package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

// healthcheckCmd performs an HTTP GET against url and exits 0 when the
// response is 2xx. It exists for container HEALTHCHECK probes: the runtime
// image is shell-less distroless, so probes must be a real binary (finding
// M17).
func healthcheckCmd(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: restitch healthcheck <url>")
		return 2
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck %s: %v\n", args[0], err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "healthcheck %s: status %d\n", args[0], resp.StatusCode)
		return 1
	}
	fmt.Fprintf(os.Stderr, "healthcheck %s: ok\n", args[0])
	return 0
}
