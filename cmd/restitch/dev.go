package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alternayte/restitch-gateway/internal/devmode"
)

const (
	devGatewayPort = 8080
	devAdminPort   = 9090
	devStudioPort  = 3080
)

func devCmd(args []string) int {
	flags := flag.NewFlagSet("dev", flag.ExitOnError)
	configFile := flags.String("config", "restitch.yaml", "path to composition config file")
	logFormat := flags.String("log-format", "text", "log format: json or text")
	studioBin := flags.String("studio-bin", "", "path to restitch-studio binary (auto-discovered if empty)")
	gatewayArgs := flags.String("gateway-args", "", "extra flags passed through to the gateway process (whitespace-separated)")
	studioArgs := flags.String("studio-args", "", "extra flags passed through to the studio process (whitespace-separated)")
	_ = flags.Parse(args)

	studioBinary, err := findStudioBinary(*studioBin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	gatewayBinary, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine executable path: %v\n", err)
		return 1
	}

	_, noColor := os.LookupEnv("NO_COLOR")
	banner := func(msg string) {
		if noColor {
			fmt.Println(msg)
		} else {
			fmt.Printf("%s%s%s\n", devmode.ColorYellow, msg, devmode.ColorReset)
		}
	}

	banner(fmt.Sprintf("restitch dev %s", version))
	banner(fmt.Sprintf("  Gateway:  http://localhost:%d", devGatewayPort))
	banner(fmt.Sprintf("  Admin:    http://localhost:%d", devAdminPort))
	banner(fmt.Sprintf("  Studio:   http://localhost:%d", devStudioPort))
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	gwOut := devmode.NewPrefixWriter(os.Stdout, "gateway", devmode.ColorCyan)
	gwErr := devmode.NewPrefixWriter(os.Stderr, "gateway", devmode.ColorCyan)
	stOut := devmode.NewPrefixWriter(os.Stdout, "studio", devmode.ColorMagenta)
	stErr := devmode.NewPrefixWriter(os.Stderr, "studio", devmode.ColorMagenta)

	gwManager := devmode.NewProcessManager(devmode.ProcessConfig{
		Name:       "gateway",
		Executable: gatewayBinary,
		Args:       buildGatewayArgs(*configFile, *logFormat, *gatewayArgs, devGatewayPort),
	}, gwOut, gwErr)

	stManager := devmode.NewProcessManager(devmode.ProcessConfig{
		Name:       "studio",
		Executable: studioBinary,
		Args:       buildStudioArgs(*studioArgs, devStudioPort, devAdminPort),
	}, stOut, stErr)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = gwManager.Run(ctx)
	}()

	healthURL := fmt.Sprintf("http://localhost:%d/health", devGatewayPort)
	banner(fmt.Sprintf("Waiting for gateway health at %s ...", healthURL))

	if err := devmode.WaitForHealth(ctx, healthURL, 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "error: gateway did not become healthy: %v\n", err)
		cancel()
		wg.Wait()
		return 1
	}
	banner("Gateway is healthy, starting studio...")

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = stManager.Run(ctx)
	}()

	sig := <-sigChan
	fmt.Println()
	banner(fmt.Sprintf("Received %v, shutting down...", sig))

	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		banner("Forced exit after timeout")
	}

	return 0
}

// buildGatewayArgs constructs the argument list for the gateway child
// process. Extra is a whitespace-separated string of additional flags
// (e.g. from --gateway-args) that is appended after the fixed arguments,
// so a duplicate flag in extra overrides the fixed one (flag package
// semantics: last occurrence wins).
func buildGatewayArgs(configFile, logFormat, extra string, port int) []string {
	args := []string{
		"run",
		"--config=" + configFile,
		"--log-format=" + logFormat,
		fmt.Sprintf("--port=%d", port),
	}
	args = append(args, strings.Fields(extra)...)
	return args
}

// buildStudioArgs constructs the argument list for the studio child
// process. Extra is a whitespace-separated string of additional flags
// (e.g. from --studio-args) that is appended after the fixed arguments,
// so a duplicate flag in extra overrides the fixed one (flag package
// semantics: last occurrence wins).
func buildStudioArgs(extra string, studioPort, adminPort int) []string {
	args := []string{
		fmt.Sprintf("--port=%d", studioPort),
		fmt.Sprintf("--gateway-admin-url=http://localhost:%d", adminPort),
	}
	args = append(args, strings.Fields(extra)...)
	return args
}

func findStudioBinary(override string) (string, error) {
	if override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("studio binary not found at %s: %w", override, err)
		}
		return override, nil
	}

	self, err := os.Executable()
	if err == nil {
		sibling := filepath.Join(filepath.Dir(self), "restitch-studio")
		if _, err := os.Stat(sibling); err == nil {
			return sibling, nil
		}
	}

	path, err := exec.LookPath("restitch-studio")
	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf("restitch-studio not found — run 'make build-all' or pass --studio-bin")
}
