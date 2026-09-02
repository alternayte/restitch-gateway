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
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v5"
)

type ProcessConfig struct {
	Name       string
	Executable string
	Args       []string
	ExtraEnv   []string
}

type ProcessManager struct {
	cfg    ProcessConfig
	stdout io.Writer
	stderr io.Writer

	initialInterval time.Duration
	maxInterval     time.Duration
	stableAfter     time.Duration

	cmdModifier func(*exec.Cmd)
}

func NewProcessManager(cfg ProcessConfig, stdout, stderr io.Writer) *ProcessManager {
	return &ProcessManager{
		cfg:             cfg,
		stdout:          stdout,
		stderr:          stderr,
		initialInterval: time.Second,
		maxInterval:     15 * time.Second,
		stableAfter:     30 * time.Second,
	}
}

func (pm *ProcessManager) Run(ctx context.Context) error {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = pm.initialInterval
	bo.MaxInterval = pm.maxInterval

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		startTime := time.Now()
		err := pm.runOnce(ctx)
		uptime := time.Since(startTime)

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			fmt.Fprintf(pm.stderr, "[%s] exited after %s: %v\n", pm.cfg.Name, uptime.Truncate(time.Millisecond), err)
		} else {
			fmt.Fprintf(pm.stderr, "[%s] exited cleanly after %s\n", pm.cfg.Name, uptime.Truncate(time.Millisecond))
		}

		if uptime >= pm.stableAfter {
			bo.Reset()
		}

		delay := bo.NextBackOff()
		fmt.Fprintf(pm.stdout, "[%s] restarting in %s...\n", pm.cfg.Name, delay.Truncate(time.Millisecond))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

func (pm *ProcessManager) runOnce(ctx context.Context) error {
	executable := pm.cfg.Executable
	if executable == "" {
		var err error
		executable, err = os.Executable()
		if err != nil {
			return fmt.Errorf("cannot determine executable path: %w", err)
		}
	}

	cmd := exec.CommandContext(ctx, executable, pm.cfg.Args...)
	cmd.Stdout = pm.stdout
	cmd.Stderr = pm.stderr

	if len(pm.cfg.ExtraEnv) > 0 {
		cmd.Env = append(os.Environ(), pm.cfg.ExtraEnv...)
	}

	if pm.cmdModifier != nil {
		pm.cmdModifier(cmd)
	}

	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 5 * time.Second

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}

	fmt.Fprintf(pm.stdout, "[%s] started (PID %d)\n", pm.cfg.Name, cmd.Process.Pid)

	return cmd.Wait()
}
