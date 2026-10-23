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
	"log/slog"
	"time"

	"github.com/alternayte/restitch-gateway/internal/admin"
)

// gatewayShutdown stops the gateway's components in a fixed order. The
// order used to follow defer registration (LIFO), which closed the store
// before the final accumulator flush.
type gatewayShutdown struct {
	// servers stop accepting traffic and drain in-flight requests, in order.
	servers []func(context.Context) error
	// stopBackground stops the registry poller, the file watcher, and the
	// periodic flusher, and returns once none of them touches the store.
	stopBackground func()
	acc            *admin.Accumulator
	store          admin.Storage
	// timeout bounds the server drain, and separately the storage flush and
	// drain (server.shutdown_timeout).
	timeout time.Duration
}

// run executes the shutdown order: servers, background work, accumulator
// flush, async store drain, storage close. It returns the server shutdown
// errors; storage errors are logged.
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

	if g.store == nil {
		return errors.Join(errs...)
	}

	storeCtx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()
	if g.acc != nil {
		for _, b := range g.acc.Flush() {
			if err := g.store.RecordBucket(storeCtx, b); err != nil {
				slog.Error("failed to flush time-series bucket on shutdown", "error", err)
			}
		}
	}
	if as, ok := g.store.(*admin.AsyncStore); ok {
		if err := as.Drain(storeCtx); err != nil {
			slog.Error("request record queue not fully drained", "error", err)
		}
	}
	if err := g.store.Close(); err != nil {
		slog.Error("failed to close storage", "error", err)
	}
	return errors.Join(errs...)
}
