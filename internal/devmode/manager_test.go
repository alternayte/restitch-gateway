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

package devmode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is not a real test — it's a child process helper.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)
	switch os.Getenv("GO_HELPER_CMD") {
	case "sleep":
		time.Sleep(time.Minute)
	case "exit1":
		fmt.Fprintln(os.Stdout, "exiting with error")
		os.Exit(1)
	case "quick":
		fmt.Fprintln(os.Stdout, "quick process done")
	default:
		fmt.Fprintf(os.Stderr, "unknown helper command: %s\n", os.Getenv("GO_HELPER_CMD"))
		os.Exit(2)
	}
}

func TestProcessManager_ContextCancel(t *testing.T) {
	stdout := &safeBuffer{}
	stderr := &safeBuffer{}

	pm := NewProcessManager(ProcessConfig{Name: "test"}, stdout, stderr)
	pm.cmdModifier = func(cmd *exec.Cmd) {
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "GO_HELPER_CMD=sleep")
	}
	pm.initialInterval = 10 * time.Millisecond
	pm.maxInterval = 50 * time.Millisecond
	pm.stableAfter = time.Second

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- pm.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("got %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	if !strings.Contains(stdout.String(), "started (PID") {
		t.Errorf("missing 'started' message: %s", stdout.String())
	}
}

func TestProcessManager_RestartOnCrash(t *testing.T) {
	stdout := &safeBuffer{}
	stderr := &safeBuffer{}

	pm := NewProcessManager(ProcessConfig{Name: "crasher"}, stdout, stderr)
	pm.cmdModifier = func(cmd *exec.Cmd) {
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "GO_HELPER_CMD=exit1")
	}
	pm.initialInterval = 10 * time.Millisecond
	pm.maxInterval = 50 * time.Millisecond
	pm.stableAfter = time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- pm.Run(ctx) }()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}

	if !strings.Contains(stdout.String(), "restarting in") {
		t.Errorf("missing 'restarting in' in stdout: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "exited after") {
		t.Errorf("missing 'exited after' in stderr: %s", stderr.String())
	}
	startCount := strings.Count(stdout.String(), "started (PID")
	if startCount < 2 {
		t.Errorf("expected >= 2 starts, got %d", startCount)
	}
}

func TestProcessManager_ExtraEnv(t *testing.T) {
	stdout := &safeBuffer{}
	stderr := &safeBuffer{}

	pm := NewProcessManager(ProcessConfig{
		Name:     "env",
		ExtraEnv: []string{"TEST_EXTRA_VAR=hello"},
	}, stdout, stderr)

	var capturedEnv []string
	pm.cmdModifier = func(cmd *exec.Cmd) {
		capturedEnv = cmd.Env
		cmd.Env = append(cmd.Env, "GO_WANT_HELPER_PROCESS=1", "GO_HELPER_CMD=quick")
	}
	pm.initialInterval = 10 * time.Millisecond
	pm.maxInterval = 50 * time.Millisecond
	pm.stableAfter = time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- pm.Run(ctx) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not complete")
	}

	found := false
	for _, env := range capturedEnv {
		if env == "TEST_EXTRA_VAR=hello" {
			found = true
			break
		}
	}
	if !found {
		t.Error("TEST_EXTRA_VAR=hello not found in cmd.Env")
	}
}
