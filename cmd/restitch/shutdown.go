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

package main

import (
	"context"
	"errors"
	"time"
)

// gatewayShutdown stops the gateway's components in a fixed order instead
// of defer order (LIFO), which once closed storage before its final flush.
type gatewayShutdown struct {
	// servers stop accepting traffic and drain in-flight requests, in order.
	servers []func(context.Context) error
	// stopBackground stops the artifact poller and the file watcher.
	stopBackground func()
	// final runs last, after traffic and background work stopped, with its
	// own deadline: the last telemetry push goes here.
	final func(context.Context)
	// timeout bounds the server drain, and separately the final step
	// (RESTITCH_SHUTDOWN_TIMEOUT).
	timeout time.Duration
}

// run executes the shutdown order: servers, background work, final step.
// It returns the server shutdown errors.
func (g *gatewayShutdown) run() error {
	var errs []error
	srvCtx, cancel := context.WithTimeout(context.Background(), g.timeout)
	for _, shutdown := range g.servers {
		if err := shutdown(srvCtx); err != nil {
			errs = append(errs, err)
		}
	}
	cancel()

	if g.stopBackground != nil {
		g.stopBackground()
	}
	if g.final != nil {
		ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
		defer cancel()
		g.final(ctx)
	}
	return errors.Join(errs...)
}
